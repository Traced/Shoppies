package utils

import (
	"encoding/base64"
	"os"
)

func ReadImageToBase64(imgPath string) (b64 string, err error) {
	data, err := os.ReadFile(imgPath)
	if err != nil {
		return b64, err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
