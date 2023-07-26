package utils

import (
	"log"
	"os"
	"path/filepath"
)

func LogFile(path, data string) bool {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("[文件日志] 创建日志目录失败！，文件：%s，数据：%s", path, data)
			return false
		}
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		log.Printf("[文件日志] 写入失败，错误：%s，文件：%s，数据：%s", err, path, data)
		return false
	}
	return true
}
