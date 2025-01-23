package utils

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

// WriteFile (覆盖)写文件
func WriteFile(filename string, data []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_SYNC|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to file: %v", err)
	}

	return nil
}

// ReadFile 读文件
func ReadFile(filename string) ([]byte, error) {
	fileContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	return fileContent, nil
}

// FileSHA256 计算文件hash值
func FileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	hashInBytes := hash.Sum(nil)[:]

	return fmt.Sprintf("%x", hashInBytes), nil
}

// DownloadFile 下载文件
func DownloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// MoveFiles 移动文件
func MoveFiles(srcDir, destDir string) error {
	files, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		srcFile := filepath.Join(srcDir, file.Name())
		destFile := filepath.Join(destDir, file.Name())

		if file.IsDir() {
			if err := os.MkdirAll(destFile, 0755); err != nil {
				return err
			}
			if err := MoveFiles(srcFile, destFile); err != nil {
				return err
			}
		} else {
			if err := os.Rename(srcFile, destFile); err != nil {
				return err
			}
		}
	}

	return nil
}

// ExtractTarGz 解压 tar.gz 文件
func ExtractTarGz(src, destDir string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}

	return nil
}

// CreateTarGz 将指定目录列表打包成 tar.gz 文件
func CreateTarGz(output string, directories []string) error {
	// 创建输出文件
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	// 创建 gzip 写入器
	gw := gzip.NewWriter(file)
	defer gw.Close()

	// 创建 tar 写入器
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, dir := range directories {
		// 遍历目录并添加文件到 tar
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// 创建 tar 头
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(path)

			// 写入头到 tar
			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// 如果是文件，写入文件内容到 tar
			if !info.IsDir() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				if _, err := io.Copy(tw, file); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
