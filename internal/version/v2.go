package version

import (
	"EMInit/pkg/utils"
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

type V2 struct {
	IFlashTool

	LocalDir        string // 本地固件目录
	RemoteUrl       string // 获取固件版本信息URL
	InitConfigUrl   string // 初始化配置文件URL
	DownloadTempDir string // 下载固件临时目录
	flashStatus     int32  // 刷写状态 0: 未进行 1: 进行中
	window          fyne.Window
}

func NewV2(flashTool IFlashTool, window fyne.Window) *V2 {
	return &V2{
		LocalDir:        "./v2_install/",
		RemoteUrl:       "https://erp.2cifang.cn/api/api/box/pkg?sn=star800",
		InitConfigUrl:   "http://box.2cifang.cn/boxinit",
		DownloadTempDir: "./tmp_v2",
		IFlashTool:      flashTool,
		window:          window,
	}
}
func (v *V2) CheckFirmwareVersion() error {
	// 创建临时下载目录
	if err := os.MkdirAll(v.DownloadTempDir, 0755); err != nil {
		return fmt.Errorf("创建临时下载目录失败: %v", err)
	}

	defer func() {
		os.RemoveAll(v.DownloadTempDir)
	}()

	firmwares, err := v.getRemoveVersion()
	if err != nil {
		return fmt.Errorf("获取固件版本信息失败: %v", err)
	}

	for _, firmware := range firmwares {
		if err := v.handleFirmwareVersion(&firmware); err != nil {
			return err
		}
	}

	return nil
}

func (v *V2) getRemoveVersion() (map[string]Firmware, error) {
	resp, err := httpClient.Get(v.RemoteUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var firmwareInfo FirmwareInfo
	if err := json.Unmarshal(body, &firmwareInfo); err != nil {
		return nil, err
	}

	firmwares := map[string]Firmware{
		"cgBox": {
			name:          "cgBox",
			localPattern:  "cgBox",
			remoteVersion: firmwareInfo.Data.CgBox.Version,
			remoteURL:     firmwareInfo.Data.CgBox.URL,
		},
	}

	return firmwares, nil
}

func (v *V2) getLocalVersion(pattern string) (int, error) {
	file := filepath.Join(v.LocalDir, fmt.Sprintf("%s.version", pattern))
	// 不存在则创建
	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			if _, err := os.Create(file); err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}

	f, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}

	if len(f) > 0 {
		var version int
		if _, err := fmt.Sscanf(string(f), "%d", &version); err == nil {
			return version, nil
		}
	}

	v.AppendOutput("未检测到本地版本信息!")
	return 0, nil
}

func (v *V2) handleFirmwareVersion(firmware *Firmware) error {
	localVersion, err := v.getLocalVersion(firmware.localPattern)
	if err != nil {
		return err
	}
	firmware.localVersion = localVersion

	if firmware.remoteVersion == firmware.localVersion {
		v.AppendOutput(fmt.Sprintf("`%s`当前已是最新版本: %d", firmware.name, localVersion))
		return nil
	}

	v.AppendOutput(fmt.Sprintf("`%s`发现新版本: %d -> %d", firmware.name, firmware.localVersion, firmware.remoteVersion))

	// 下载固件
	downloadPath := filepath.Join(v.DownloadTempDir, fmt.Sprintf("%s-%d.tar.gz", firmware.name, firmware.remoteVersion))
	if err := utils.DownloadFile(firmware.remoteURL, downloadPath); err != nil {
		return fmt.Errorf("下载`%s`失败: %v", firmware.name, err)
	}

	// 创建目录并解压固件
	extractDir := filepath.Join(v.DownloadTempDir, fmt.Sprintf("%s-%d", firmware.name, firmware.remoteVersion))
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("创建解压目录失败: %v", err)
	}
	if err := utils.ExtractTarGz(downloadPath, extractDir); err != nil {
		return fmt.Errorf("解压`%s`失败: %v", firmware.name, err)
	}

	// 移动文件
	if err := utils.MoveFiles(extractDir, v.LocalDir); err != nil {
		return fmt.Errorf("移动文件失败: %v", err)
	}

	// 记录版本信息
	err = os.WriteFile(filepath.Join(v.LocalDir, fmt.Sprintf("%s.version", firmware.localPattern)), []byte(fmt.Sprintf("%d", firmware.remoteVersion)), 0644)
	if err != nil {
		return fmt.Errorf("记录版本信息失败: %v", err)
	}

	v.AppendOutput(fmt.Sprintf("`%s`升级成功!", firmware.name))

	return nil
}

func (v *V2) packageFirmware(needRePackage bool) (err error) {
	v.AppendOutput("开始打包固件...")

	tarFile := "v2_init.tar.gz"
	if needRePackage {
		os.Remove(tarFile)
		v.AppendOutput("删除旧固件重新打包...")
	}

	if _, err := os.Stat(tarFile); err == nil {
		v.AppendOutput("固件已存在，无需再次打包!")
		time.Sleep(3 * time.Second)
		return nil
	}

	// 打包文件
	dirs := []string{
		"v2_install",
		"share",
	}

	if err = utils.CreateTarGz(tarFile, dirs); err != nil {
		return err
	}

	return nil
}

func (v *V2) DownloadConfig(sn string) error {
	url := fmt.Sprintf("%s/%s", v.InitConfigUrl, sn)

	response, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	v.AppendOutput(fmt.Sprintf("开始下载配置文件，请求URL: %s", url))

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("响应码：%v", response.StatusCode)
	}

	var setting struct {
		Success int    `json:"success"`
		Msg     string `json:"msg"`
	}
	data, err := io.ReadAll(response.Body)
	err = json.Unmarshal(data, &setting)
	if err != nil || setting.Success != 1 {
		return fmt.Errorf("未获取到设备相关配置文件: %s", setting.Msg)
	}

	_ = os.MkdirAll("setting/v2", 0755)
	err = utils.WriteFile(fmt.Sprintf("setting/v2/%s.json", sn), data)
	return err
}

func (v *V2) FlashFirmware(devSN string, rePackage bool) {
	if devSN == "" {
		dialog.ShowInformation("警告", "设备SN不能为空!", v.window)
		return
	}

	if !atomic.CompareAndSwapInt32(&v.flashStatus, 0, 1) {
		v.AppendOutput("刷写正在进行中!")
		dialog.ShowInformation("警告", "刷写正在进行中!", v.window)
		return
	}

	conf := dialog.NewConfirm("确认初始化", fmt.Sprintf("您确定要初始化 %s 吗？", devSN), func(confirmed bool) {
		if !confirmed {
			atomic.StoreInt32(&v.flashStatus, 0)
			return
		}

		v.AppendOutput("开始刷写v2程序，执行过程请勿关闭程序!")

		go func() {
			var err error

			if err = v.packageFirmware(rePackage); err != nil {
				v.AppendOutput("打包固件失败: " + err.Error())
				return
			}
			v.AppendOutput("打包固件完成！")

			defer func() {
				atomic.StoreInt32(&v.flashStatus, 0)
				if err != nil {
					v.AppendOutput("刷写固件失败: " + err.Error())
				} else {
					v.AppendOutput("刷写固件成功!")
				}
			}()

			// 删除临时目录
			if _, err = v.RunAndWaitCommand("rm -rf /tmpcf"); err != nil {
				return
			}
			// 创建临时目录
			if _, err = v.RunAndWaitCommand("mkdir -p /tmpcf"); err != nil {
				return
			}

			// 传输文件
			if err = v.UploadFile("v2_init.tar.gz", "/tmpcf/v2_init.tar.gz"); err != nil {
				return
			}
			if err = v.UploadFile("v2_install.sh", "/tmpcf/v2_install.sh"); err != nil {
				return
			}

			// 执行脚本
			if _, err = v.RunAndWaitCommand(fmt.Sprintf("cd /tmpcf && sh v2_install.sh %s", devSN)); err != nil {
				return
			}

			// 尝试将初始配置文件传到设备
			if err = v.UploadFile(fmt.Sprintf("setting/v2/%s.json", devSN), "/datas/cf_go_v2/data/init/setting.json"); err == nil {
				v.AppendOutput("初始配置文件上传成功!")
			} else {
				v.AppendOutput("初始配置文件上传失败: " + err.Error())
				err = nil
				v.AppendOutput("设备插卡联网后，将自动更新初始配置文件!")
			}

		}()

	}, v.window)

	conf.SetConfirmText("是")
	conf.SetDismissText("否")
	conf.Show()
	return
}
