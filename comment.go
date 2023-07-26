package main

// 可以通过店铺ID找到店铺商品列表
//
// 每个商品页面地址除了商品 ID 不同

type (
	Shop struct {
		// 店铺地址
		URL string
		// 店铺唯一 ID
		ID string
		// 店铺商品留言
		Comments
	}
	Comment struct {
		ProductID string
		// ProductURL string
		Content  string
		GuestURL string
		Date     string
	}
	Comments []Comment
)

func main() {

}
