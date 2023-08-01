package accounts

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	jsoniter "github.com/json-iterator/go"

	"Shoppies/utils"
	"gitee.com/baixudong/gospider/requests"
)

var (
	// 所有产品所在目录
	productPath                = "products"
	siteURL                    = "https://shoppies.jp"
	matchProductTotalNumRegexp = regexp.MustCompile(`(\d+)件 \d+/(\d+)`)
	matchShopItemIDRegexp      = regexp.MustCompile(`user-item/(\d+)`)
	// YesCaptcha 实例
	/*
		YesCaptcha = utils.NewYesCaptcha(
			"c595d17eb0cf028366bb60210c502acf85cfe6f814774",
			"RecaptchaV3TaskProxyless",
			"https://shoppies.jp/",
			"6LewTP8UAAAAAI3855Ww2s7yBxkXrdvNOJo2ycKC")
		_ = YesCaptcha
	*/
)

func getDefaultHeaders() map[string]string {
	return map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":           "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7",
		"Cache-Control":             "no-cache",
		"Connection":                "keep-alive",
		"DNT":                       "1",
		"Pragma":                    "no-cache",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
		"User-Agent":                "Mozilla/5.0 (Linux; Android 8.0.0; SM-G955U Build/R16NW) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.141 Mobile Safari/537.36",
		"sec-ch-ua":                 `Not.A/Brand";v="8", "Chromium";v="114", "Google Chrome";v="114"`,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        "\"macOS\"",
	}
}

func NewAccount(username, password, proxyAddr string) *Account {
	c, _ := requests.NewClient(nil, requests.ClientOption{
		Ja3:     true,
		H2Ja3:   true,
		TryNum:  3,
		Headers: getDefaultHeaders(),
	})

	if proxyAddr != "" {
		_ = c.SetProxy(proxyAddr)
	}

	return &Account{
		Username: username,
		Password: password,
		Http:     c,
	}
}

type Account struct {
	Username string           `json:"username"`
	Password string           `json:"password"`
	IsLogin  bool             `json:"-"`
	Http     *requests.Client `json:"-"`
}

// SetProxy 设置这个账号使用的代理
func (a *Account) SetProxy(proxyAddr string) error {
	return a.Http.SetProxy(proxyAddr)
}

// GetProxyIP 查询当前账号使用的IP
func (a *Account) GetProxyIP() string {
	resp, err := a.Http.Get(nil, "http://ip-api.com/json")
	if err != nil {
		log.Println("[获取IP] 获取失败：", err)
		return ""
	}
	r, _ := resp.Json()
	return r.Get("query").Str
}

// Login 对账号进行登录操作
//
// 这里有坑，手机端页面跟电脑端获取的数据不一样！,这里做手机端登录
func (a *Account) Login() error {
	a.IsLogin = false
	loginPageURL := fmt.Sprintf("%s/user-login_test/", siteURL)
	resp, err := a.Http.Get(nil, loginPageURL)
	if err != nil {
		log.Printf("[登录] 账号 %s 登录失败，访问登录页获取值失败：%s\n", a.Username, err)
		return loginNetworkError
	}
	form := resp.Html().Find("[action*=\"ses_id\"]")
	// 获取表单
	if form == nil {
		logFile := "log/" + a.Username + ".login.html"
		utils.LogFile(logFile, resp.Text())
		log.Printf("[登录] 账号 %s 登录失败，访问登录页获取值失败,已记录在 %s\n", a.Username, logFile)
		return errors.New("[登录] 登录失败，解析登录表单失败！")
	}
	// 重新提交数据到登录网站，以post的方式,ses_id 从表单获取
	payload := map[string]string{
		"loginforid": "1",
		"loginid":    a.Username,
		"password":   a.Password,
		"loginbtn":   "ログイン",
	}
	// 这个 id 时有时无的，有的话就一起提交
	sessID := form.Find("[name=\"ses_id\"]")
	if sessID != nil {
		payload["ses_id"] = sessID.Get("value")
	}
	resp, err = a.Http.Post(nil, fmt.Sprintf("%s%s", siteURL, form.Get("action")),
		requests.RequestOption{
			Data: payload,
		})
	if err != nil {
		log.Printf("[登录] 账号 %s 登录失败, 可能存在网络问题：", err)
		return loginNetworkError
	}
	if resp.Html().Find("[name=\"auto_login_flag\"]") != nil {
		log.Printf("[登录] 账号 %s 登录失败, 可能是密码错误或者账号已经不存在了。\n", a.Username)
		return AccountError
	}
	log.Printf("[登录] 账号 %s 登录成功。\n", a.Username)
	a.IsLogin = true
	return nil
}

// GetProductNumAndTotalPage 获取商品总数和总分页数
func (a *Account) GetProductNumAndTotalPage() (num, totalPage int) {
	if !a.IsLogin {
		log.Printf("[获取商品总数] 账号 %s 获取失败，请先登录！\n", a.Username)
		return
	}
	resp, err := a.Http.Get(nil, fmt.Sprintf("%s/index.php?jb=member-item_user_list", siteURL),
		requests.RequestOption{
			Headers: getDefaultHeaders(),
		})
	if err != nil {
		log.Printf("[获取商品总数] 账号 %s 获取失败，可能是网络问题？：%s\n", a.Username, err)
		return
	}
	numBar := resp.Html().Find(".tempPageTitleBar")
	if nil == numBar {
		logFilepath := "log/product.num." + a.Username + ".html"
		utils.LogFile(logFilepath, resp.Text())
		log.Printf("[获取商品总数] 账号 %s 获取失败，没有获取到总数，已记录到：%s\n", a.Username, logFilepath)
		return
	}
	// 总数和分页数在这个元素上面
	matchs := matchProductTotalNumRegexp.FindAllStringSubmatch(numBar.Text(), 2)
	// 把字符串转换成数字
	num, _ = strconv.Atoi(matchs[0][1])
	totalPage, _ = strconv.Atoi(matchs[0][2])
	return
}

// GetAllProductIDs 获取所有产品ID
func (a *Account) GetAllProductIDs() (ids []string) {
	if !a.IsLogin {
		log.Printf("[获取商品总数] 账号 %s 获取失败，请先登录！\n", a.Username)
		return
	}
	num, totalPage := a.GetProductNumAndTotalPage()
	if 1 > num {
		return
	}
	var (
		wg sync.WaitGroup
	)
	wg.Add(totalPage)
	log.Printf("[获取商品ID] 账号 %s，商品总数：%d，总分页数：%d", a.Username, num, totalPage)
	for ; totalPage > 0; totalPage-- {
		go func(i int) {
			log.Printf("[获取商品ID] 账号 %s 启动线程获取商品ID，线程编号： %d\n", a.Username, i)
			defer wg.Done()
			// https://shoppies.jp/index.php?jb=member-item_user_list&page=
			pageURL := fmt.Sprintf("%s/index.php?jb=member-item_user_list&page=%d", siteURL, i)
			resp, err := a.Http.Get(nil, pageURL)
			if err != nil {
				log.Printf("[获取商品ID] 账号 %s 获取所有商品ID，线程 %d 出现网络错误：%s\n", a.Username, i, err)
				return
			}
			// utils.LogFile("log/id.html", resp.Text())
			// 没找到
			itemList := resp.Html().Finds(`[name="itemidarray[]"]`)
			if 1 > len(itemList) {
				return
			}
			for _, item := range itemList {
				ids = append(ids, item.Get("value"))
			}
		}(totalPage)
	}
	wg.Wait()
	return ids
}

// DeleteAllProduct 删除所有商品
//
// 返回被删除的商品数量
func (a *Account) DeleteAllProduct() int {
	if !a.IsLogin {
		log.Printf("[清空所有商品] 账号 %s 删除商品失败，请先登录！\n", a.Username)
		return 0
	}
	var (
		itemIDs = a.GetAllProductIDs()
		total   = len(itemIDs)
	)
	if 1 > total {
		log.Printf("[清空所有商品] 账号 %s 商品数量为 0 不需要清空！\n", a.Username)
		return 0
	}
	log.Printf("[清空所有商品] 账号 %s 共获取到 %d 个商品，正在执行删除操作！\n", a.Username, total)
	_, err := a.Http.Post(nil, fmt.Sprintf("%s/index.php?jb=delete-item_ar_end", siteURL),
		requests.RequestOption{
			Data: map[string][]string{
				"itemidarray[]": itemIDs,
			},
		})
	if err != nil {
		log.Printf("[清空所有] 账号 %s 删除商品失败，可能存在网络错误：%s！\n", a.Username, err)
		return 0
	}
	log.Printf("[清空所有商品] 账号 %s 删除商品成功，共删除 %d 个商品！\n", a.Username, total)
	return total
}

func (a *Account) LoginAndDeleteProduct() (err error) {
	err = a.Login()
	if err == nil {
		a.DeleteAllProduct()
		return
	}
	return
}

type UploadImageResponse struct {
	Url   string `json:"image_url"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// UploadImage 上传商品图片
//
// index: 0 - 3
// imgPath: 图片路径
func (a *Account) UploadImage(index, imgPath string) (ir UploadImageResponse) {
	uploadURL := fmt.Sprintf("%s/api/putImageForSP.php", siteURL)
	b64, err := utils.ReadImageToBase64(imgPath)
	if err != nil {
		log.Printf("[上传图片] 账号 %s 上传图片失败，图片资源问题：%s\n", a.Username, err)
		return
	}
	resp, err := a.Http.Post(nil, uploadURL, requests.RequestOption{
		Data: map[string]string{
			"block":   index,
			"dataUrl": "data:image/jpeg;base64," + b64,
		},
	})
	if err != nil {
		log.Printf("[上传图片] 账号 %s 图片上传失败，网络问题：%s\n", a.Username, err)
		return
	}
	r, _ := resp.Json()

	ir.Name = r.Get("result.name").Str
	ir.Url = r.Get("result.image_url").Str
	ir.Error = r.Get("result.error").Str
	return
}

// UploadImages 快速上传图片
//
// 使用多线程上传
func (a *Account) UploadImages(paths [][2]string) (successfulList []string, failureList [][2]string) {
	var (
		wg    sync.WaitGroup
		order [4]string
	)
	wg.Add(len(paths))
	for _, info := range paths {
		go func(i [2]string) {
			defer wg.Done()
			log.Printf("[上传商品图片] 账号 %s 正在上传商品图片 %s\n", a.Username, i[1])
			ir := a.UploadImage(i[0], i[1])
			// 上传成功
			if ir.Error == "" {
				// 多线程可能会导致拼接顺序不一样
				// 这个网站不是以上传时候的block为顺序点
				idx, _ := strconv.Atoi(i[0])
				order[idx] = ir.Name
				return
			}
			failureList = append(failureList, i)
			log.Printf("[上传商品图片] 账号 %s 上传商品 %s 图片失败：%s\n", a.Username, i[1], ir.Error)
		}(info)
	}
	wg.Wait()
	// 利用数组特性，让无序变有序
	successfulList = order[:]
	return
}

// SlowUploadImages 一张张的上传图片
func (a *Account) SlowUploadImages(paths [][2]string) (successfulList []string, failureList [][2]string) {
	for _, i := range paths {
		log.Printf("[上传商品图片] 账号 %s 正在上传商品图片 %s\n", a.Username, i[1])
		ir := a.UploadImage(i[0], i[1])
		// 上传成功
		if ir.Error == "" {
			successfulList = append(successfulList, ir.Name)
			continue
		}
		failureList = append(failureList, i)
		log.Printf("[上传商品图片] 账号 %s 上传商品 %s 图片失败：%s\n", a.Username, i[1], ir.Error)
	}
	return
}

// UploadImageForProduct 上传指定商品目录下的商品图片
func (a *Account) UploadImageForProduct(product *ProductConfig) error {
	productDir := productPath + "/" + product.ID
	files, err := os.ReadDir(productDir)
	if err != nil {
		e := errors.New(fmt.Sprintf("[上传商品图片] 账号 %s 上传图片失败，商品 %s 目录读取出错：%s", a.Username, product.ID, err))
		log.Println(e)
		return e
	}
	var imgPaths [][2]string
	// 过滤jpg图片，path 格式 [图片块索引，图片路径]
	for _, file := range files {
		if n, ok := strings.CutSuffix(file.Name(), ".jpg"); ok {
			imgPaths = append(imgPaths, [2]string{n, productDir + "/" + n + ".jpg"})
		}
	}
	// 开始上传
	successful, failure := a.UploadImages(imgPaths)
	// 图片名字使用,进行拼接，表单格式如此
	product.PictureURL = strings.Join(successful, ",")
	// 上传成功的数量
	product.SuccessfulCount = len(successful)
	// 上传失败的图片名字列表
	product.FailureList = failure
	log.Printf("[上传商品图片] 账号 %s 商品 %s 共成功上传 %d 张图片, 失败 %d 张\n",
		a.Username, product.ID, product.SuccessfulCount, len(failure))
	return nil
}

type (
	ProductConfig struct {
		// 用于标识商品唯一的文件夹名字
		ID string `json:"-"`
		// 加载产品配置错误
		Error error `json:"-"`
		// 上传成功的数量
		SuccessfulCount int `json:"-"`
		// 上传失败的图片列表, [图片索引，图片路径]，记录是为了方便后续做失败重传的功能
		FailureList [][2]string `json:"-"`
		// 商品图片名字
		PictureURL   string `json:"-"`
		Price        string `json:"price"`
		CategoryName string `json:"category_name"`
		CategoryID   string `json:"category_id"`
		// 商品说明
		Explanation string `json:"explanation"`
		// 商品名字
		Title string `json:"title"`
		// 配送方式
		CarryMethod string `json:"carry_method"`
	}
)

// ReadProductConfig 读取商品配置
func (a *Account) ReadProductConfig(i string) (pc ProductConfig) {
	pc.ID = i
	configJson, err := os.ReadFile(productPath + "/" + i + "/config.json")
	if err != nil {
		errTips := fmt.Sprintf("[读取商品配置] 账号 %s 读取商品 %s 配置出错：%s\n", a.Username, i, err)
		pc.Error = errors.New(errTips)
		log.Println(errTips)
		return
	}
	// 载入配置文件出错，可能是json配置没写好
	if err = jsoniter.Unmarshal(configJson, &pc); err != nil {
		errTips := fmt.Sprintf("[读取商品配置] 账号 %s 载入商品 %s 配置出错：%s\n", a.Username, i, err)
		pc.Error = errors.New(errTips)
		log.Println(errTips)
	}
	return
}

// PublishProduct 发布商品
//
// 使用指定商品数据进行填充并发布
func (a *Account) PublishProduct(product *ProductConfig) bool {
	// YesCaptcha.PageAction = "write/item"
	if !a.IsLogin {
		log.Printf("[发布商品] 账号 %s 发布失败，请先登录！\n", a.Username)
		return false
	}
	if product.PictureURL == "" {
		log.Printf("[发布商品] 账号 %s 发布失败，缺少商品图片！\n", a.Username)
		return false
	}
	// 从配置文件里读取 价格、类别、产品说明、商品名字，配送方式其他都是可以固定的
	payload := map[string]string{
		// 从网页上获取的值
		// "carry_condition": resp.Html().Find(`[id="carry_condition"]`).Text(),
		// 这些值都是需要从配置文件里读取的
		"category_id":   product.CategoryID,
		"category_name": product.CategoryName,
		"input_price":   product.Price,
		"explanation":   product.Explanation,
		"title":         product.Title,
		"carry_method":  product.CarryMethod,

		// 这个是读取配置文件之后上传图片后返回的
		"picture_url":     product.PictureURL,
		"carry_condition": "",
		// 这些值都是可以固定的
		"item_genre_id":         "",
		"item_genre_details_id": "",
		"carry_fee_type":        "1",
		"tagkey":                "",
		"viewtype":              "new",
		"template_id":           "",
		"carry_method_flag":     "1",
		"send_date_standard":    "3",
		"rot":                   "1",
		"no_price_flag":         "0",
		"btn1":                  "1",
		"area":                  "北海道",
		"carry_id":              "1",
		"brand_id":              "",
		"atr_status":            "1",
		"shop_category_id":      "",
		"item_stocks":           "1",
		"chakuga_flag":          "0",
		"atr_size":              "",
		"total_mid":             "1",
		"private_flag":          "0",

		// 因为有部分字段提交时候重合，估计后端只是取有需要的字段，因此直接追加进去
		// 这部分是确认出品时候提交的数据
		"pics":          "",
		"brand_name":    "",
		"itemnum":       "",
		"comment":       "",
		"option1_title": "",
		"option1_value": "",
		"option2_title": "",
		"option2_value": "",
		"option3_title": "",
		"option3_value": "",
		"option4_title": "",
		"option4_value": "",
		"option5_title": "",
		"option5_value": "",
	}
	newHeaders := getDefaultHeaders()
	// 提交商品切换成电脑版 UA
	newHeaders["User-Agent"] = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/55.0.2883.75 Safari/537.36"
	// 开始提交商品
	log.Println("[发布商品] 账号", a.Username, "提交数据至：", siteURL+"/write-item_conf")
	resp, err := a.Http.Post(nil, siteURL+"/write-item_conf", requests.RequestOption{
		Data:    payload,
		Headers: newHeaders,
	})
	if err != nil {
		log.Printf("[发布商品] 账号 %s 出品失败，网络错误：%s！\n", a.Username, err)
		return false
	}
	form := resp.Html()
	log.Printf("[发布商品] 账号 %s 正在确认出品！\n", a.Username)

	confValue := form.Find("[name=\"conf\"]")
	// 出品失败，网站提示压力大
	if confValue == nil {
		// 查找警告提示
		warning := form.Find(".newsDetailsText")
		if warning == nil {
			pageLogFile := "log/write-item_conf." + a.Username + "-" + product.ID + ".html"
			utils.LogFile(pageLogFile, resp.Text())
			log.Printf("[发布商品] 账号 %s 确认出品失败，没找到conf值，未知原因，已记录页面信息：%s！\n", a.Username, pageLogFile)
			return false
		}
		// 警告内容
		warningText := strings.TrimSpace(warning.Text())
		product.Error = errors.New(warningText)
		log.Printf("[发布商品] 账号 %s 确认出品编号 %s 失败，警告内容: %s\n", a.Username, product.ID, warningText)
		return false
	}
	payload["conf"] = confValue.Get("value")
	log.Println("[发布商品] 账号", a.Username, "提交数据至：", siteURL+"/index_pc.php?jb=write-item_end")
	resp, err = a.Http.Post(nil, siteURL+"/index_pc.php?jb=write-item_end", requests.RequestOption{
		Data:    payload,
		Headers: newHeaders,
	})
	if err != nil {
		log.Printf("[发布商品] 账号 %s 确认出品失败，网络错误：%s！\n", a.Username, err)
		return false
	}
	// 出品失败可能是需要过验证码，当前版本没有过谷歌验证码
	if !strings.Contains(resp.Text(), "出品が完了") {
		logFilepath := "log/write.end" + product.ID + "." + a.Username + ".html"
		utils.LogFile(logFilepath, resp.Text())
		log.Printf("[发布商品] 账号 %s 确认出品失败，失败页面已记录到文件：%s！\n", a.Username, logFilepath)
		return false
	}
	log.Printf("[发布商品] 账号 %s 确认出品成功！\n", a.Username)
	return true
}

func (a *Account) LogFailedReason(err error) {
	// 写入加锁：多线程可能会同时写一个文件
	writeBadLock.Lock()
	defer writeBadLock.Unlock()
	// 一行一个：用户名 密码 错误信息
	line := []byte("\n" + a.Username + " " + a.Password + "" + err.Error() + "\n")
	utils.WriteFile(badReasonFilepath, line, os.O_APPEND)
}

type (
	TodoItem struct {
		Title   string `json:"title"`
		Message string `json:"msg"`
		Date    string `json:"date"`
		Picture string `json:"picture"`
		Url     string `json:"url"`
	}
	TodoList []*TodoItem
)

// CheckAlive 检测账号是否可用
//
// 有两种情况：一种登录失败，另一种不可购买（购买页面出现警告提示）
func (a *Account) CheckAlive() (err error) {
	log.Printf("[可用检测] 开始检测账号 %s 的可用性\n", a.Username)
	// 账号还没登录，先登录一下
	if !a.IsLogin {
		log.Printf("[可用检测] 账号 %s 进行登录\n", a.Username)
		// 登录失败
		if err = a.Login(); errors.Is(err, AccountError) {
			a.LogFailedReason(errors.New("登录异常"))
			log.Printf("[可用检测] 账号 %s 不可用: %s", a.Username, err)
			return AccountError
		}
	}

	// 检测待办事项中是否有完成电子邮件验证提示
	resp, err := a.Http.Get(nil, siteURL+"/?jb=member-todo_list")
	if err != nil {
		log.Printf("[可用检测] 账号 %s 访问待办事项页面失败：/?jb=member-todo_list 发生网络错误：%s\n", a.Username, err)
	} else {
		if strings.Contains(resp.Text(), "認証を完了") {
			a.LogFailedReason(errors.New("未完成邮箱认证"))
			log.Printf("[可用检测] 账号 %s 不可用：待办事项检测到未完成邮箱认证", a.Username)
			return AccountError
		}
	}

	itemID := "000"
	// 去分类随便获取一个商品id，用于检测没有认证的情况
	resp, err = a.Http.Get(nil, siteURL+"/user-item_list/cid-0109/p/39")
	if err != nil {
		log.Printf("[可用检测] 账号 %s 访问主页获取商品ID失败，发生网络错误：%s\n", a.Username, err)
	} else {
		matched := matchShopItemIDRegexp.FindStringSubmatch(resp.Text())
		// 这里也可能会没有，为了防止异常报错导致退出
		// 还是记录下什么情况吧，方便后续查看分析
		if 2 > len(matched) {
			log.Printf("[可用检测] 账号 %s 访问主页获取商品ID失败，没找到商品，已记录：%s ", a.Username, "log/item-list."+a.Username+".html")
			utils.LogFile("log/item-list."+a.Username+".html", resp.Text())
		} else {
			itemID = matched[1]
		}
	}

	checkBadPageURL := fmt.Sprintf("%s/index_pc.php?jb=member-order&id=%s", siteURL, itemID)
	log.Printf("[可用检测] 账号 %s 访问订单页面：%s\n", a.Username, checkBadPageURL)
	resp, err = a.Http.Get(nil, checkBadPageURL)
	if err != nil {
		log.Printf("[可用检测] 账号 %s 检测失败，不作判断。或可能存在网络错误：%s\n", a.Username, err)
		return
	}

	// 跳转到情报设定页面
	if strings.HasSuffix(resp.Url().String(), "jb=edit-info") {
		// 记录到日志
		a.LogFailedReason(errors.New("未设置会员信息"))
		return AccountError
	}

	// 除了警告提示 其他都不影响
	errTips := resp.Html().Find(".cborder_pink")
	if errTips != nil && strings.Contains(errTips.Text(), "警告中") {
		a.LogFailedReason(errors.New("购买页面中出现警告"))
		log.Printf("[可用检测] 账号 %s 不可用：购买页面中出现警告", a.Username)
		return AccountError
	}
	log.Printf("[可用检测] 账号 %s 可用！", a.Username)
	return
}
