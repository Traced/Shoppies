package tools

import (
	"Shoppies/utils"
	"fmt"
	"gitee.com/baixudong/gospider/requests"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	proxyAddr     = "socks5://xp112233_area-JP:xiaopao0o0@43.128.63.227:7710"
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

func Start() {
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
