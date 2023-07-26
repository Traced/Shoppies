package main

import (
	"os"

	jsoniter "github.com/json-iterator/go"

	"Shoppies/utils"
)

type Config struct {
	Price        string `json:"price"`
	CategoryName string `json:"category_name"`
	CategoryId   string `json:"category_id"`
	Explanation  string `json:"explanation"`
	Title        string `json:"title"`
	CarryMethod  string `json:"carry_method"`
}

func main() {
	root := "." // 当前目录
	entries, err := os.ReadDir(root)
	if err != nil {
		panic(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirName := entry.Name()
			configPath := dirName + "/config.json"
			_, err := os.Stat(configPath)
			if !os.IsNotExist(err) {
				configByteData, err := os.ReadFile(configPath)
				if err == nil {
					var conf Config
					jsoniter.Unmarshal(configByteData, &conf)
					conf.Title += dirName
					data, _ := jsoniter.MarshalIndent(conf, "", "  ")
					utils.WriteFile(configPath, data, 0)
				}
			}
		}
	}
}
