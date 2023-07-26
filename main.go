package main

import (
	jsoniter "github.com/json-iterator/go"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"Shoppies/accounts"
	"Shoppies/utils"
	"gitee.com/baixudong/gospider/requests"
)

var (
	rangeFilepath     = "products/range.txt"
	accountFilepath   = "account.txt"
	runConfigFilepath = "run.config.json"

	// 一行一个的配置文件中使用的字段分隔
	configFileSeparate = "----"
)

type (
	RunConfig struct {
		MaxSuccessAttempts int `json:"max_success_attempts"`
		Minute             int `json:"minute"`
		Seconds            int `json:"seconds"`
		Retry              int `json:"retry"`
		Interval           int `json:"interval"`
	}
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime | log.Lmicroseconds)
	utils.MkdirAll("log/aa")
	// 设置同时写日志到控制台和文件
	if f, err := os.OpenFile("log/run.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	}

	// 运行任务
	//RunTasks()
	Test()
}

func Test() {
	var wg sync.WaitGroup
	wg.Add(30)
	for i := 0; 30 > i; i++ {
		go func() {
			defer wg.Done()
			accounts.PopupAccount()
		}()
	}
	wg.Wait()
}

func RunTasks() {
	rangeBytesData, err := os.ReadFile(rangeFilepath)
	if err != nil {
		log.Println("[初始化] range.txt 读取失败：", err)
		return
	}
	runConfigBytesData, err := os.ReadFile(runConfigFilepath)
	if err != nil {
		log.Println("[初始化] range.txt 读取失败：", err)
		return
	}
	var runConfig RunConfig

	if jsoniter.Unmarshal(runConfigBytesData, &runConfig) != nil {
		log.Println("[初始化] run.config.json 读取失败！")
		return
	}
	var (
		tasks accounts.TaskList
		// 一行一个
		ranges = strings.Split(string(rangeBytesData), "\n")
		total  = len(ranges)
		// 读取总账号库
		mainAccountCount, _ = utils.CountNonEmptyLines(accountFilepath)
	)

	log.Println("[初始化] 从 range.txt 读取到：", total, "个任务分段，总账号库账号数量：", mainAccountCount)

	for id, r := range ranges {
		rs := strings.Split(strings.TrimSpace(r), configFileSeparate)
		if 3 > len(rs) {
			continue
		}

		var (
			// 把读取到的字符出转成数字
			// 开始、结束、循环任务账号总数
			start, _            = strconv.Atoi(rs[0])
			end, _              = strconv.Atoi(rs[1])
			totalLoopAccount, _ = strconv.Atoi(rs[2])

			// 循环库索引
			loopAccountFilename = "ranges/range_" + strconv.Itoa(id) + ".txt"
			// 读取循环库
			taskAccountList, err = utils.ReadFileAtNonEmptyLines(loopAccountFilename)
		)

		if err != nil {
			utils.CreateFile(loopAccountFilename, nil)
			log.Printf("[初始化] 不存在分段 %d 循环任务库，建立库：%s", id, loopAccountFilename)
		}

		var (
			// 循环库已有的账号数量
			totalTaskAccount = len(taskAccountList)
			// 补充的账号数量
			chargeNum = totalLoopAccount - totalTaskAccount
			// 待充值的账号列表
			chargeAccountList []string
		)

		// 判断循环库账号数量是否满足设定的数量
		// 不满足数量就从总账号库中补充缺口数量
		// 总账号库还有账号
		for ; chargeNum > 0 && mainAccountCount > 0; mainAccountCount-- {
			username, password := accounts.PopupAccount()
			// 读取失败，有可能读到空行
			if username == "" {
				continue
			}
			// 读取成功，需要补充的数量减一
			chargeAccountList = append(chargeAccountList, username+configFileSeparate+password)
			chargeNum--
		}

		// 将补充的账号写入循环库
		if len(chargeAccountList) > 0 {
			// 合并补充的账号到已有的账号列表
			// 这么做是因为下方还要分配一个账号来初始化
			// 同时避免了读取到空索引导致的报错
			taskAccountList = append(taskAccountList, chargeAccountList...)
			log.Printf("[初始化] 补充 %d 个账号到循环任务库文件 %s", len(chargeAccountList), loopAccountFilename)
			// 最后以换行符结束，下次写文件就不用重新换行了
			utils.WriteFile(loopAccountFilename, []byte(strings.Join(chargeAccountList, "\n")+"\n"), os.O_APPEND)
		}

		// 为任务分配代理
		// proxyAddr := accounts.PopupProxyAddr()
		// 从循环账号中取第一个账号出来编排任务: 账号 密码
		account := strings.Split(taskAccountList[0], configFileSeparate)
		// 加入到任务管理器中
		tasks = append(tasks, accounts.NewTask(
			id, runConfig.MaxSuccessAttempts, totalTaskAccount,
			runConfig.Minute, runConfig.Seconds, runConfig.Retry, runConfig.Interval,
			loopAccountFilename, accounts.TaskRange{start, end},
			account[0], account[1]))
	}

	// 开始任务，多线程
	tasks.Run()
}

func CheckAccountAlive() {
	checkFile := "account.txt"
	// 检测死活
	if len(os.Args) > 1 {
		checkFile = os.Args[1]
	}

	lines, err := utils.ReadFileAtLines(checkFile)
	log.Printf("读取到%d个账号等待检测", len(lines))
	if err != nil {
		log.Println("读取文件错误：", err)
		return
	}
	BatchCheckAlive(lines)
}

func BatchCheckAlive(accountList []string) {
	var wg sync.WaitGroup
	wg.Add(len(accountList))
	for _, account := range accountList {
		go func(acc string) {
			defer wg.Done()
			// 账号密码用空格分开
			info := strings.Split(strings.TrimSpace(acc), configFileSeparate)
			a := accounts.NewAccount(info[0], info[1], "")
			var status string
			if a.CheckAlive() == nil {
				status = "良好"
			} else {
				status = "不可用"
			}
			log.Printf("账号 %s 可用状态为：%s\n", info[0], status)
		}(account)
	}
	wg.Wait()
}

func BatchDeleteProduct(accountList []string) {
	var wg sync.WaitGroup
	wg.Add(len(accountList))
	for _, account := range accountList {
		go func(acc string) {
			defer wg.Done()
			a := accounts.NewAccount(acc, "z123456", "socks5://127.0.0.1:2021")
			if a.Login() == nil {
				a.DeleteAllProduct()
			}
		}(account)
	}
	wg.Wait()
}

var (
	// siteURL = "https://baidu.com"
	// siteURL = "http://httpbin.org/ip"
	siteURL = "https://ip.useragentinfo.com/json"
	// siteURL           = "https://shoppies.jp"
	testHttpClient, _ = requests.NewClient(nil, requests.ClientOption{
		Headers: map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
			"Accept-Language":           "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7",
			"Cache-Control":             "no-cache",
			"Connection":                "keep-alive",
			"DNT":                       "1",
			"Pragma":                    "no-cache",
			"Sec-Fetch-Dest":            "document",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-Site":            "same-origin",
			"Sec-Fetch-User":            "?1",
			"Upgrade-Insecure-Requests": "1",
			"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36",
			"sec-ch-ua-mobile":          "?0",
		},
	})
)

func CheckProxy(testURL string, proxyAddrList []string) {
	var wg sync.WaitGroup
	wg.Add(len(proxyAddrList))
	for _, proxyAddr := range proxyAddrList {
		go func(addr string) {
			addr = strings.TrimSpace(addr)
			defer wg.Done()
			log.Println("开始测试代理：", addr)
			resp, err := testHttpClient.Get(nil, testURL, requests.RequestOption{
				Proxy:   addr,
				Timeout: 5 * time.Second,
			})
			if err != nil {
				log.Println("[代理检测]", addr, "不可用：", err)
				return
			}
			if resp.StatusCode() != 200 {
				log.Println("[代理检测]", addr, "不可用,请检查测试地址是否返回了200状态码："+testURL)
				return
			}
			log.Println("[代理检测]", addr, "可用:", resp.Text())
		}(strings.TrimSpace(proxyAddr))
	}
	wg.Wait()
}
