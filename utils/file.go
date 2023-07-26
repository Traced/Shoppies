package utils

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// CountNonEmptyLines 统计所有非空行
func CountNonEmptyLines(filename string) (int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var count int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// ReadFileAtLine 读取文件指定行
func ReadFileAtLine(filename string, lineNumber int) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var line string
	for i := 1; scanner.Scan(); i++ {
		if i == lineNumber {
			line = scanner.Text()
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if line == "" {
		return "", fmt.Errorf("line %d not found", lineNumber)
	}
	return line, nil
}

// CutFileAtNonEmptyLine 从文件中剪切一行非空行
func CutFileAtNonEmptyLine(filename string) (line string, err error) {
	lines, err := CutFileAtNonEmptyLines(filename, 1)
	if err != nil {
		return "", err
	}
	// 文件内容行为 0
	if 1 > len(lines) {
		return
	}
	line = lines[0]
	return
}

// CutFileAtNonEmptyLines 从文件中剪切 n 行非空行
func CutFileAtNonEmptyLines(filename string, n int64) (lines []string, err error) {
	lines, err = ReadFileAtNonEmptyLines(filename)
	if err != nil {
		return
	}
	// 文件内容行为 0
	if n > int64(len(lines)) {
		return
	}
	// 将剩余的行写回文件
	WriteFile(filename, []byte(strings.Join(lines[n:], "\n")+"\n"), os.O_TRUNC)
	return
}

// ReadFileAtLines 读取所有行
func ReadFileAtLines(filepath string) (lines []string, err error) {
	dataBytes, err := os.ReadFile(filepath)
	return strings.Split(strings.TrimSpace(string(dataBytes)), "\n"), err
}

// ReadFileAtNonEmptyLines 读取非空行
func ReadFileAtNonEmptyLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// CreateFile 创建一个文件
func CreateFile(filepath string, data []byte) (err error) {
	file, err := os.Create(filepath)
	if err != nil {
		log.Printf("[创建文件] %s 创建失败！", filepath)
		return
	}
	defer file.Close()
	if len(data) > 0 {
		file.Write(data)
	}
	return err
}

// MkdirAll 创建不存在的路径
func MkdirAll(path string) error {
	path = filepath.Dir(path)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return err
}

// WriteFile 写入文件
func WriteFile(filepath string, data []byte, appendFlag int) {
	// 创建不存在的目录
	_ = MkdirAll(filepath)

	file, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|appendFlag, 0644)
	if err != nil {
		log.Println("[写入文件] 打开失败：", filepath, err)
		return
	}
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		log.Println("[写入文件] 写入失败：", filepath, err)
		return
	}
}

// ModifyLineFromFile 修改文件的某一行内容
func ModifyLineFromFile(filepath string, lineNumber int, newContent string) error {
	file, err := os.OpenFile(filepath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if lineNumber < 1 || lineNumber > len(lines) {
		return fmt.Errorf("invalid line number")
	}

	// Modify the content of the specified line
	lines[lineNumber-1] = newContent

	// Truncate the file to 0 length and write the modified lines back to the file
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// RemoveLineFromFile 删除指定行并返回
func RemoveLineFromFile(filename string, lineNumber int) (string, error) {
	// 打开原始文件
	file, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 计算指定行的偏移量和长度
	var offset int64
	var length int64
	scanner := bufio.NewScanner(file)
	var i int
	for scanner.Scan() {
		i++
		if i == lineNumber {
			lineBytes := scanner.Bytes()
			length = int64(len(lineBytes))
			if length > 0 && lineBytes[length-1] == '\n' {
				length-- // 如果最后一个字节是换行符，就不将其包含在删除的内容中
			}
			break
		}
		offset += int64(len(scanner.Bytes()) + 1) // 加上行尾符的长度
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// 将指定行的内容保存下来，并替换为空字符串
	lineBytes := make([]byte, length)
	if _, err := file.ReadAt(lineBytes, offset); err != nil {
		return "", err
	}
	line := string(lineBytes)
	if _, err := file.WriteAt([]byte(""), offset); err != nil {
		return "", err
	}

	// 向前移动文件内容
	if _, err := file.Seek(offset+int64(length)+1, 0); err != nil {
		return "", err
	}
	buffer := make([]byte, 4096)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if _, err := file.WriteAt(buffer[:n], offset); err != nil {
				return "", err
			}
			offset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	// 截断文件并保存修改
	if err := file.Truncate(offset); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	return line, nil
}
