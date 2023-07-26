package accounts

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"Shoppies/utils"
	"gitee.com/baixudong/gospider/requests"
)

func NewTask(id, maxSuccessAttempts, taskAccountNum, minute, seconds, retry, interval int, taskAccountFilepath string, ranges TaskRange, username, password string) *Task {
	return &Task{
		ID:                  id,
		TaskAccountFilepath: taskAccountFilepath,
		// 一轮任务中最大允许发布成功的数量
		// 到达后该数量后该轮将不再执行任务，等待下一轮
		MaxSuccessAttempts: maxSuccessAttempts,
		Ranges:             ranges,
		LoopAccountNum:     taskAccountNum,
		// 当前账号在循环库中的位置
		AccountIndex: 0,
		Account:      NewAccount(username, password, ""),
		// 传品失败重试次数，3次过后就等待下一轮
		RetryTimes: retry,
		// 间隔三秒
		PublishInterval: interval,
		// 工作流程开始执行的时间
		StartMinute:  minute,
		StartSeconds: seconds,
	}
}

var (
	accountFilepath   = "account.txt"
	proxyAddrFilepath = "proxy.txt"
	badReasonFilepath = "bad.error.txt"

	// 一行一个的配置文件中使用的字段分隔
	configFileSeparate = "----"

	EmptyAccountError   = errors.New("[更换新账号] 更换失败，请检查账号库存！")
	EmptyProxyAddrError = errors.New("[更换新代理] 更换失败，请检查代理库存！")
	loginNetworkError   = errors.New("登录失败，网络错误！")
	loginError          = errors.New("登录失败！")
	AccountError        = errors.New("账号错误！")

	writeBadLock     sync.Mutex
	popupAccountLock sync.Mutex
)

// PopupAccount 从账号库文件从取出账号，并且从账号库中删除已经取出来的账号
func PopupAccount() (username, password string) {
	// 多线程读取加锁
	popupAccountLock.Lock()
	defer popupAccountLock.Unlock()

	// 读取文件非空行
	line, err := utils.CutFileAtNonEmptyLine(accountFilepath)
	if err != nil {
		log.Println("[读取账号库] 失败：", err)
		return
	}

	// 从总账号库读到 0 行
	if line == "" {
		log.Println("[读取账号库] 失败：总帐号库存不足！")
		return
	}

	// 分割账号密码
	account := strings.Split(strings.TrimSpace(line), configFileSeparate)
	// 读取不到账号
	if 2 > len(account) {
		log.Printf("[读取账号库] 失败：读取到的账号格式有误：%s！", line)
		return
	}

	return account[0], account[1]
}

// PopupProxyAddr 从代理库文件从取出账号，并且从库中删除已经取出来的代理
func PopupProxyAddr() (proxyAddr string) {
	// return "socks5://wkkk852_area-JP:971030@proxy.smartproxycn.com:1000"
	//return "socks5://wkkk852_area-JP:971030@43.128.63.227:7710"
	// _life-30_session-abcdefghi
	//return "socks5://xp112233_area-JP:xiaopao0o0@43.128.63.227:7710"
	proxyAddr, err := utils.RemoveLineFromFile(proxyAddrFilepath, 1)
	if err != nil {
		log.Println("[读取代理库] 失败：", err)
		return
	}
	return strings.TrimSpace(proxyAddr)
}

// Run 并发启动所有任务
func (tl TaskList) Run() {
	if 1 > len(tl) {
		return
	}

	var wg sync.WaitGroup
	for _, task := range tl {
		wg.Add(1)
		go func(t *Task) {
			defer wg.Done()
			t.Run()
		}(task)
	}
	wg.Wait()
}

type (
	TaskRange [2]int
	TaskList  []*Task
	Task      struct {
		// 任务 ID
		ID int
		// 任务使用的账号库
		TaskAccountFilepath string
		// 这个任务使用的ip代理
		proxyAddr string
		// 这个任务负责的商品编号范围：开始 - 结束。单个账号任务
		Ranges TaskRange
		// 用于循环的账号数量
		LoopAccountNum int
		// 这个任务使用的账号,更换账号直接重置下所使用的账号密码
		*Account
		// 账号在循环库中的索引
		AccountIndex int
		// 这个任务目前进度
		ProgressIndex int
		// 上传失败重试次数
		RetryTimes int
		// 已经重试了几次
		retryCount int
		// 上传间隔
		PublishInterval int

		// 本轮最大允许成功数
		MaxSuccessAttempts int
		// 已经成功次数
		SuccessCount int

		// 程序执行时间: 开始工作流程的时间
		StartMinute  int
		StartSeconds int
	}
)

func (t *Task) ReadTaskProxyIP() string {
	ip, _ := utils.ReadFileAtLine("proxy.txt", t.ID+1)
	t.proxyAddr = ip
	return ip
}

// Run 开始执行任务
func (t *Task) Run() {
	// 每小时几分开始执行任务
	minute, sec, hourAfter := t.StartMinute, t.StartSeconds, 1
	//minute, sec, hourAfter := time.Now().Minute(), time.Now().Second()+3, 0
	log.Println("[执行任务] 任务启动 -", t.ID, "-", t.Username,
		"任务分段：", t.Ranges,
		"使用代理：", t.ReadTaskProxyIP(),
		"每小时", minute, "分，",
		sec+t.PublishInterval, "秒开始执行任务，重试次数：", t.RetryTimes,
		"商品发布间隔：", t.PublishInterval)

	// 获取当前时间
	now := time.Now()
	// 计算下一个小时的准点时间
	nextHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+hourAfter, minute, sec, 0, now.Location())
	// 计算当前时间和下一个小时准点时间的时间差
	waitDuration := nextHour.Sub(now)
	// 创建定时器
	ticker := time.NewTicker(waitDuration)
	defer ticker.Stop()

	// 设置开始编号
	t.ProgressIndex = t.Ranges[0]

	// 第一次运行程序时登录并删除商品
	_ = t.LoginAndDeleteProduct()

	// 开始循环
	for {
		select {
		// 到时间后才开始执行任务
		case <-ticker.C:
			// 在开始本轮时，初始化本轮成功数量
			t.SuccessCount = 0
			loginError := t.TryLogin()
			// 执行任务时候先登录
			if loginError == nil {
				log.Printf("%d 任务：%v  使用ip：%s 使用账号：%s", t.ID, t.Ranges, t.proxyAddr, t.Username)
				// 执行任务
				t.Execute()
				log.Printf("任务线程 %d：%v %s 本次任务已结束，等待下次执行，当前账号商品总数量：%d", t.ID, t.Ranges, t.Username, len(t.GetAllProductIDs()))
			} else if errors.Is(loginError, EmptyAccountError) {
				// 账号库存不足
				log.Println("请补充总账号库存！")
				// 总账号库没号了，就结束掉任务
				//return
			} else if errors.Is(loginError, EmptyProxyAddrError) {
				// 代理库存不足
				log.Println("请补充代理库存！")
				// 代理库没代理了，也结束掉任务
				// 因为可能影响到后续操作
				//return
			}
			// 重新计算下一个小时的准点时间和等待时间
			now = time.Now()
			nextHour = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, minute, sec, 0, now.Location())
			waitDuration = nextHour.Sub(now)
			ticker.Reset(waitDuration)
		}
	}
}

// ChangeProxy 更换代理
func (t *Task) ChangeProxy() error {
	proxyAddr := PopupProxyAddr()
	// 获取失败
	if proxyAddr == "" {
		log.Println(EmptyProxyAddrError)
		return EmptyProxyAddrError
	}
	// 代理设置失败后会有报错
	return t.SetProxyAddr(proxyAddr)
}

// TryLogin 尝试登录执行任务，遇到账号被封自动更换下一个账号
func (t *Task) TryLogin() error {
	t.Http.Close()
	// 每一轮使用新的客户端
	t.Account = NewAccount(t.Username, t.Password, t.ReadTaskProxyIP())
	log.Printf("%d 任务：%v 读取ip：%s ", t.ID, t.Ranges, t.proxyAddr)

	// 检查账号是否可用，不可用自动补充：对于没登陆的账号会自动自动
	// 补充失败
	err := t.CheckAliveAndSupplement()
	if err != nil {
		log.Printf("[执行任务-登录-补充账号] 补充失败，请检查总账号库！")
		return err
	}

	return err
}

// AccountSupplement 补充账号到循环库文件
//
// 从总账号库抽取一个账号追加到当前任务循环库末尾
func (t *Task) AccountSupplement() error {
	// 从总账号读取一个账号
	username, password := PopupAccount()
	if strings.TrimSpace(username) == "" {
		log.Printf("[补充账号] 任务 %d %v 补充账号失败：总账号库异常！", t.ID, t.Ranges)
		return EmptyAccountError
	}
	// 追加到最后
	utils.WriteFile(t.TaskAccountFilepath, []byte(username+configFileSeparate+password+"\n"), os.O_APPEND)
	return nil
}

// NextAccount 切换到下一个账号
//
// 将当前账号放入循环库末尾，取第一个账号并进行初始化设置
func (t *Task) NextAccount() error {
	line, _ := utils.CutFileAtNonEmptyLine(t.TaskAccountFilepath)
	// 读取完，把账号追加到循环库最后一行
	utils.WriteFile(t.TaskAccountFilepath, []byte(line+"\n"), os.O_APPEND)
	// 读取下一个账号
	line, _ = utils.ReadFileAtLine(t.TaskAccountFilepath, 1)
	account := strings.Split(strings.TrimSpace(line), configFileSeparate)
	if 2 > len(account) {
		log.Printf("[换号] 任务线程 %d，换号失败，读取到的账号无效！", t.ID)
		// 换号失败返回账号错误
		return AccountError
	}
	log.Printf("[换号] 任务线程 %d 换至 %s", t.ID, line)
	// 任务重新开始
	t.ProgressIndex = t.Ranges[0]
	// 初始化账号状态
	t.Username, t.Password, t.IsLogin, t.retryCount = account[0], account[1], false, 0
	t.SuccessCount = 0
	t.Http.ClearCookies()

	// 递归检测账号死活
	if err := t.CheckAliveAndSupplement(); err != nil {
		return err
	}
	return nil
}

// Execute 执行任务
func (t *Task) Execute() {
	// 延迟设定的秒数后开始执行任务
	log.Printf("[执行任务-上传图片] 任务线程 %d 将在 %d 秒后开始执行\n", t.ID, t.PublishInterval)
	// 读取商品配置文件
	pc := t.ReadProductConfig(strconv.Itoa(t.ProgressIndex))
	// 读取配置文件失败，不执行
	if pc.Error != nil {
		return
	}
	// 上传商品图片
	// 上传失败 或者 一张都没上传上去
	if nil != t.UploadImageForProduct(&pc) || 1 > pc.SuccessfulCount {
		// 如果需要做失败的图片重传，可以判断下...
		// 但是由于时间关系，先不做这个
		// if len(pc.FailureList)>0{
		//	t.UploadImages(pc.FailureList)
		// }
		return
	}
	time.Sleep(time.Duration(t.PublishInterval) * time.Second)
	// 商品信息发布成功
	if t.PublishProduct(pc) {
		// 成功发布商品，下一轮就传别的
		t.ProgressIndex++
		// 如果商品全部传完了，就切换到下一个号，继续执行任务
		if t.ProgressIndex >= t.Ranges[1] {
			log.Printf("[执行任务-发布商品信息] 账号 %s 已完成任务，切换到下一个账号继续执行！", t.Username)
			// 切换失败，读取到的账号有问题
			// 这个都是读取循环库第一个账号
			// 读取失败有两个主要原因：总账号库空了、账号格式不对
			// 这两种情况无论是哪一种，都得等下一个任务时间点
			// 因为在触发定时任务时做了错误判断，所以可以直接结束掉本轮任务执行
			if t.NextAccount() != nil {
				// 检测通过，删除所有商品
				t.DeleteAllProduct()
				return
			}
			log.Printf("[执行任务-发布商品信息] 切换账号至 %s 开始任务，任务 ID： %d！", t.Username, t.ID)
		}

		// 如果设定了最大成功次数
		if t.MaxSuccessAttempts > 0 {
			// 本轮发布成功计数
			t.SuccessCount++
			log.Printf("[执行任务-发布商品信息] 任务线程 %d 账号 %s 本轮已完成 %d 次任务！", t.ID, t.Username, t.SuccessCount)
			// 如果已经完成了设定好的次数
			// 结束当前轮任务
			if t.SuccessCount >= t.MaxSuccessAttempts {
				return
			}
		}

		// 继续下一个商品的发布
		t.Execute()
		return
	}

	/* 失败重试部分
	*  发布失败了，验证下账号可用状态，如果账号是可用的，那么重试
	 */

	// 如果重试次数达到了设定好的次数但还是失败，这一轮也不试了
	if t.retryCount >= t.RetryTimes {
		log.Printf("[执行任务-发布商品] 发布失败，账号 %s 重试 %d 次后仍然失败，等待下一轮！\n", t.Username, t.RetryTimes)
		// 重置尝试次数，避免下一轮无法进行重试
		t.retryCount = 0
		return
	}
	// 重试次数 + 1
	t.retryCount++

	// 还没重试的话就验证下可用性
	// 如果账号可用的话，说明这小时已经不能上传了
	if 1 > t.retryCount {
		// 检查账号可用性并尝试替换
		// 如果账号不可用或者补充失败，就不重试了
		if t.CheckAliveAndSupplement() == nil {
			t.retryCount = 0
			return
		}
		log.Println("[执行任务-发布商品] 发布失败，账号存在问题，已换号")
	}
	log.Printf("[执行任务-发布商品] 发布失败，账号 %s 正在重试第 %d/%d 次\n", t.Username, t.retryCount, t.RetryTimes)
	t.Execute()
	return
}

// CheckAliveAndSupplement 检查账号可用性，不可用自动从总账号库补充一个账号
func (t *Task) CheckAliveAndSupplement() error {
	// 如果是账号错误，那么从总账号库补充一个新账号
	if err := t.CheckAlive(); errors.Is(err, AccountError) {
		// 记录账号不可用原因
		t.LogFailedReason(err)

		// 删除循环库第一行账号
		_, _ = utils.CutFileAtNonEmptyLine(t.TaskAccountFilepath)
		// 补充账号
		sErr := t.AccountSupplement()
		if sErr == nil {
			return t.NextAccount()
		}
		// 发现账号库空了，把错误抛出给上一层处理
		if errors.Is(sErr, EmptyAccountError) {
			return sErr
		}
		return err
	}
	return nil
}

// SetAccount 设置新账号
//
// 设置传入的账号密码为当前使用的账号并进行登录后删除所有商品
func (t *Task) SetAccount(username, password string) error {
	// 任务重新开始
	t.ProgressIndex = t.Ranges[0]
	// 初始化账号状态
	t.Username, t.Password, t.IsLogin, t.retryCount = username, password, false, 0
	t.Http.ClearCookies()
	return t.LoginAndDeleteProduct()
}

// SetProxyAddr 设置当前任务使用的代理
func (t *Task) SetProxyAddr(addr string) error {
	t.proxyAddr = addr
	return t.Http.SetProxy(addr)
}

// CheckProxyAvailable 检查当前代理是否可用
func (t *Task) CheckProxyAvailable(addr string) bool {
	return true
	// 传空就是检测当前设置的代理
	if addr == "" {
		addr = t.proxyAddr
	}
	resp, err := t.Http.Head(nil, siteURL, requests.RequestOption{
		Proxy:   addr,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		log.Println("[代理检测]", addr, "不可用：", err)
		return false
	}
	if resp.StatusCode() != 200 {
		log.Println("[代理检测]", addr, "不可用,请检查测试地址是否返回了200状态码："+siteURL)
		return false
	}
	log.Println("[代理检测]", addr, "可用")
	return true
}
