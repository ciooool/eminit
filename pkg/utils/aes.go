package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

func EncryptAES256(plaintext, key, iv string) (string, error) {
	keyBytes, err := generateAES256Key(key)
	if err != nil {
		return "", err
	}

	return EncryptAES(plaintext, string(keyBytes), iv)
}

func DecryptAES256(plaintext, key, iv string) (string, error) {
	keyBytes, err := generateAES256Key(key)
	if err != nil {
		return "", err
	}

	return DecryptAES(plaintext, string(keyBytes), iv)
}

func generateAES256Key(key string) ([]byte, error) {
	hash := sha256.New()
	_, err := io.WriteString(hash, key)
	if err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// EncryptAES 加密
func EncryptAES(plaintext, key, iv string) (string, error) {
	plaintextBytes := []byte(plaintext)

	// 创建 AES 加密算法实例
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	// 对明文信息进行填充
	blockSize := block.BlockSize()
	plaintextBytes = PKCS5Padding(plaintextBytes, blockSize)

	// 创建 CBC 分组模式实例
	mode := cipher.NewCBCEncrypter(block, []byte(iv))

	// 加密数据
	ciphertext := make([]byte, len(plaintextBytes))
	mode.CryptBlocks(ciphertext, plaintextBytes)

	// 将加密后的数据进行 base64 编码
	ciphertextBase64 := base64.StdEncoding.EncodeToString(ciphertext)
	return ciphertextBase64, nil
}

// DecryptAES 解密
func DecryptAES(ciphertextBase64, key, iv string) (string, error) {
	// 解密数据
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", err
	}

	// 创建 AES 加密算法实例
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	// 创建 CBC 分组模式实例
	mode := cipher.NewCBCDecrypter(block, []byte(iv))

	// 解密数据
	plaintextBytes := make([]byte, len(ciphertextBytes))
	mode.CryptBlocks(plaintextBytes, ciphertextBytes)

	// 去除填充
	plaintextBytes, err = PKCS5UnPadding(plaintextBytes)
	return string(plaintextBytes), err
}

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS5UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)

	// 确保 origData 不为空
	if length == 0 {
		return nil, errors.New("empty input")
	}
	unPadding := int(origData[length-1])

	// 确保 unPadding 的值在有效范围内
	if unPadding > length {
		return nil, errors.New("invalid unPadding value")
	}

	return origData[:(length - unPadding)], nil
}
