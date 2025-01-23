package version

import (
	"crypto/tls"
	"net/http"
)

type IFlashTool interface {
	// RunAndWaitCommand 运行并等待命令执行结果
	RunAndWaitCommand(cmd string) (string, error)
	// AppendOutput 输出日志
	AppendOutput(message string)
	// UploadFile 上传文件
	UploadFile(localPath, remotePath string) error
}

type IFirmwareVersion interface {
	// CheckFirmwareVersion 检查固件版本
	CheckFirmwareVersion() error
	// FlashFirmware 刷写固件
	FlashFirmware(devSN string, rePackage bool)
	// DownloadConfig 下载网关配置
	DownloadConfig(sn string) error
}

type Firmware struct {
	name          string
	localPattern  string
	localVersion  int
	remoteVersion int
	remoteURL     string
}

type FirmwareInfo struct {
	Success int `json:"success"`
	Data    struct {
		CgManager struct {
			URL     string `json:"url"`
			Version int    `json:"version"`
		}
		CgService struct {
			URL     string `json:"url"`
			Version int    `json:"version"`
		}
		CgProtocol struct {
			URL     string `json:"url"`
			Version int    `json:"version"`
		}
		CgBox struct {
			URL     string `json:"url"`
			Version int    `json:"version"`
		}
	} `json:"data"`
}

var httpClient *http.Client

func init() {
	httpClient = &http.Client{
		Transport: &http.Transport{
			// 忽略证书
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}
