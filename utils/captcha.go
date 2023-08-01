package utils

import (
	"context"
	"log"
	"net/url"
	"time"

	"gitee.com/baixudong/gospider/requests"
)

func NewYesCaptcha(apiKey, typ, siteURL, siteKey string) *YesCaptcha {
	return &YesCaptcha{ApiKey: apiKey, Type: typ, SiteURL: siteURL, SiteKey: siteKey}
}

var (
	yesCaptchaApiURL, _ = url.Parse("https://api.yescaptcha.com")
	http, _             = requests.NewClient(nil, requests.ClientOption{
		TryNum: 3,
		OptionCallBack: func(_ context.Context, option *requests.RequestOption) error {
			// 相对路径加上 url
			if !option.Url.IsAbs() {
				option.Url = yesCaptchaApiURL.ResolveReference(option.Url)
			}
			return nil
		},
	})
)

type (
	YesCaptcha struct {
		ApiKey     string
		Type       string
		SiteURL    string
		SiteKey    string
		PageAction string
	}
	YesCaptchaTaskResponse struct {
		ErrorID   int    `json:"errorId"`
		ErrorCode string `json:"errorCode"`
		ErrorMsg  string `json:"errorDescription"`
		TaskID    string `json:"taskId"`
		Solution  *struct {
			// 谷歌验证码响应
			Result string `json:"gRecaptchaResponse"`
		} `json:"solution"`
		Status  string `json:"status"`
		Balance uint64 `json:"balance"`
		Error   error  `json:"-"`
	}
)

func (y *YesCaptcha) CreateTask() (r YesCaptchaTaskResponse) {
	resp, err := http.Post(nil, "/createTask", requests.RequestOption{
		Json: map[string]any{
			"clientKey": y.ApiKey,
			"task": map[string]string{
				"type":       y.Type,
				"websiteURL": y.SiteURL,
				"websiteKey": y.SiteKey,
				"pageAction": y.PageAction,
			},
		},
	})
	if err != nil {
		r.Error = err
		return
	}
	if _, err = resp.Json(&r); err != nil {
		log.Println("[创建打码任务] 出现错误：", err, &r)
		r.Error = err
	}
	log.Println("[创建打码任务] 创建成功，响应：", resp.Text())
	return
}

func (y *YesCaptcha) GetResult(taskResponse *YesCaptchaTaskResponse) *YesCaptchaTaskResponse {
	// 创建任务时候发生错误
	if taskResponse.Error != nil {
		log.Println("[获取打码结果] 提交的任务响应出现错误：", taskResponse.Error)
		return taskResponse
	}
	// 开始等待结果
	for {
		resp, err := http.Post(nil, "/getTaskResult", requests.RequestOption{
			Json: map[string]string{
				"clientKey": y.ApiKey,
				"taskId":    taskResponse.TaskID,
			},
		})
		// 打码出现网络错误
		if err != nil {
			log.Println("[获取打码结果] 出现错误：", err)
			time.Sleep(time.Second * 5)
			continue
		}
		_, _ = resp.Json(&taskResponse)
		// 出错了：当errorId 大于0，请根据errorDescription了解出错误信息
		if taskResponse.ErrorID > 0 {
			log.Println("[获取打码结果] 识别出现错误：", taskResponse.ErrorMsg)
			break
		}
		// 识别成功：当errorId等于0 并且status等于 ready，结果在solution里面。
		if taskResponse.Status == "ready" {
			log.Println("[获取打码结果] 识别已完成，结果：", taskResponse.Solution)
			break
		}
		log.Println("[获取打码结果] 等待打码平台返回识别结果...")
		// 正在识别中：当errorId等于0 并且status等于 processing，请3秒后重试。
		time.Sleep(time.Second * 5)
	}
	return taskResponse
}

// BypassRecaptcha 过 reCAPTCHA
func (y *YesCaptcha) BypassRecaptcha() *YesCaptchaTaskResponse {
	tr := y.CreateTask()
	return y.GetResult(&tr)
}

// Balance 查询余额
func (y *YesCaptcha) Balance() uint64 {
	resp, err := http.Post(nil, "/getBalance", requests.RequestOption{
		Json: map[string]string{
			"clientKey": y.ApiKey,
		},
	})
	if err != nil {
		log.Println("[获取打码平台余额] 获取失败，可能是网络错误：", err)
		return 0
	}
	var r YesCaptchaTaskResponse
	_, _ = resp.Json(&r)
	return r.Balance
}
