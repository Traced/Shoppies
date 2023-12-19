package tools

import (
	"Shoppies/accounts"
	"Shoppies/utils"
	"errors"
	"fmt"
	"github.com/gospider007/requests"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	//proxyAddr     = "socks5://127.0.0.1:1080"
	proxyAddr     = ""
	httpClient, _ = requests.NewClient(nil, requests.ClientOption{
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
		Proxy: proxyAddr,
	})
	matchProductNumRegexp = regexp.MustCompile(`検索結果.+?(\d+)件`)
)

type (
	TaskFunc func() (keep bool)
)

var (
	configFileSeparate = "----"
)

func StartCheckLoginTask() {
	utils.SetLogOutputFile("log/check-login.log.txt")
	checkFile, proxyAddr := "account.txt", ""
	if len(os.Args) > 1 {
		checkFile = os.Args[1]
	}
	if len(os.Args) > 2 {
		proxyAddr = os.Args[2]
	}

	lines, err := utils.ReadFileAtLines(checkFile)
	log.Printf("读取到%d个账号等待检测", len(lines))
	if err != nil {
		log.Println("读取文件错误：", err)
		return
	}

	for _, account := range lines {
		// 账号密码用空格分开
		info := strings.Split(strings.TrimSpace(account), configFileSeparate)
		a := accounts.NewAccount(info[0], info[1], proxyAddr)
		var status string
		if err = a.Login(); errors.Is(err, accounts.AccountError) {
			status = "不可用"
		} else {
			status = "良好"
		}
		log.Printf("账号 %s 可用状态为：%s\n", info[0], status)
	}
}

// 统计指定链接列表商品数量
func taskCountProductNum() (keep bool) {
	utils.SetLogOutputFile("log/product.log.txt")
	var (
		linksFilepath = "product.links.txt"
		logFilepath   = "product.num.result.txt"
		links, err    = utils.ReadFileAtNonEmptyLines(linksFilepath)
		wg            sync.WaitGroup
		results       = []string{
			"",
			time.Now().Format("2006-01-02 15:04:05"),
		}
	)

	if err != nil {
		log.Printf("文件[%s]读取失败:%s", linksFilepath, err)
		return
	}

	log.Printf(results[1])
	wg.Add(len(links))
	for _, link := range links {
		go func(l string) {
			defer wg.Done()
			resp, err := httpClient.Get(nil, l)
			if err != nil {
				log.Printf("访问[%s]失败:%s", l, err)
				return
			}
			if resp.StatusCode() != 200 {
				log.Printf("访问[%s]失败,获取内容失败:%s", l, err)
				return
			}
			targetTag := resp.Html().Find(".searchResultCnt")
			if targetTag == nil {
				log.Printf("获取总数[%s]失败,未找到标记元素！", l)
				return
			}
			log.Printf(targetTag.Text())
			matched := matchProductNumRegexp.FindStringSubmatch(targetTag.Text())
			if 1 > len(matched) {
				log.Printf("获取总数[%s]失败,未找到匹配到数据！", l)
				return
			}
			results = append(results, fmt.Sprintf("%s ---- %s", l, matched[1]))
		}(link)
	}
	wg.Wait()

	utils.WriteFile(logFilepath, []byte(strings.Join(results, "\n")+"\n"), os.O_APPEND)

	return true
}

func StartCountTask() {
	m, s := 52, 30
	if len(os.Args) > 1 {
		m, _ = strconv.Atoi(os.Args[1])
	}
	if len(os.Args) > 2 {
		s, _ = strconv.Atoi(os.Args[2])
	}
	RunTicker(m, s, taskCountProductNum)
}

func RunTicker(minute, sec int, task TaskFunc) {
	if task == nil {
		return
	}
	log.Printf("每个小时 %d 分钟 %d 秒开始执行任务", minute, sec)
	// 获取当前时间
	now := time.Now()
	// 计算下一个小时的准点时间
	nextHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), minute, sec, 0, now.Location())
	// 计算当前时间和下一个小时准点时间的时间差
	waitDuration := nextHour.Sub(now)
	// 创建定时器
	ticker := time.NewTicker(waitDuration)
	defer ticker.Stop()

	// 开始循环
	for {
		select {
		// 到时间后才开始执行任务
		case <-ticker.C:
			if !task() {
				return
			}
			// 重新计算下一个小时的准点时间和等待时间
			now = time.Now()
			nextHour = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, minute, sec, 0, now.Location())
			waitDuration = nextHour.Sub(now)
			ticker.Reset(waitDuration)
		}
	}
}
