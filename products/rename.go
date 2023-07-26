package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	orderBy := "min"
	if len(os.Args) > 1 {
		orderBy = os.Args[1]
	}

	if orderBy != "dir" {
		if orderBy == "max" {
			println("从大到小排序命名")
		} else {
			println("从小到大排序命名")
		}
	}
	// 获取当前程序执行的文件夹路径
	folderPath, _ := os.Getwd()

	var dirs []string
	// 获取文件夹下所有子文件夹的路径
	subFolders, _ := os.ReadDir(folderPath)
	di := 0
	for _, subFolder := range subFolders {
		if subFolder.IsDir() {
			subFolderPath := subFolder.Name()
			if orderBy != "dir" {
				// 追加文件夹
				dirs = append(dirs, subFolderPath)

				// 获取子文件夹下所有以数字命名的图片文件
				imgFiles, _ := os.ReadDir(subFolderPath)
				var imgFileNames []string

				for _, imgFile := range imgFiles {
					if _, ok := strings.CutSuffix(imgFile.Name(), ".jpg"); ok && !imgFile.IsDir() {
						imgFileNames = append(imgFileNames, imgFile.Name())

						oldName := subFolderPath + "/" + imgFile.Name()
						newName := subFolderPath + "/" + imgFile.Name() + ".bak.jpg"
						os.Rename(oldName, newName)
					}
				}
				// 按文件名数字
				sort.Slice(imgFileNames, func(i, j int) bool {
					num1, _ := strconv.Atoi(strings.TrimSuffix(imgFileNames[i], ".jpg"))
					num2, _ := strconv.Atoi(strings.TrimSuffix(imgFileNames[j], ".jpg"))
					// 从大到小排序
					if orderBy == "max" {
						return num1 > num2
					}
					// 从小到大排序
					return num2 > num1
				})

				// 重命名图片文件
				for i, imgFileName := range imgFileNames {
					oldName := subFolderPath + "/" + imgFileName + ".bak.jpg"
					newName := subFolderPath + "/" + strconv.Itoa(i+1) + ".jpg"
					os.Rename(oldName, newName)
				}
				fmt.Printf("%s 下的图片已重命名完成\n", subFolderPath)
			} else {
				os.Rename(subFolderPath, strconv.Itoa(di))
				di++
			}
		}
	}
}
