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
	"regexp"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/util/gconv"
)

type V3 struct {
	IFlashTool

	LocalDir        string // 本地固件目录
	RemoteUrl       string // 获取固件版本信息URL
	InitConfigUrl   string // 初始化配置文件URL
	DownloadTempDir string // 下载固件临时目录
	flashStatus     int32  // 刷写状态 0: 未进行 1: 进行中

	window fyne.Window
}

func NewV3(flashTool IFlashTool, window fyne.Window) *V3 {
	return &V3{
		LocalDir:        "./v3_install/bin/",
		RemoteUrl:       "https://erp.2cifang.cn/api/api/box/pkg?sn=arm7v3",
		InitConfigUrl:   "https://erp.2cifang.cn/api/box/erp",
		DownloadTempDir: "./tmp_v3",
		IFlashTool:      flashTool,
		window:          window,
	}
}

// CheckFirmwareVersion 检查固件版本
func (v *V3) CheckFirmwareVersion() error {
	// 创建临时下载目录
	if err := os.MkdirAll(v.DownloadTempDir, 0755); err != nil {
		return fmt.Errorf("创建临时下载目录失败: %v", err)
	}

	defer func() {
		os.RemoveAll(v.DownloadTempDir)
	}()

	firmwares, err := v.getRemoteVersion()
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

func (v *V3) getRemoteVersion() (map[string]Firmware, error) {
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
		"cgManager": {
			name:          "cgManager",
			localPattern:  "cgManager_main",
			remoteVersion: firmwareInfo.Data.CgManager.Version,
			remoteURL:     firmwareInfo.Data.CgManager.URL,
		},
		"cgService": {
			name:          "cgService",
			localPattern:  "cgService",
			remoteVersion: firmwareInfo.Data.CgService.Version,
			remoteURL:     firmwareInfo.Data.CgService.URL,
		},
		"cgProtocol": {
			name:          "cgProtocol",
			localPattern:  "cgProtocol",
			remoteVersion: firmwareInfo.Data.CgProtocol.Version,
			remoteURL:     firmwareInfo.Data.CgProtocol.URL,
		},
	}

	return firmwares, nil
}

func (v *V3) getLocalVersion(pattern string) (int, error) {
	files, err := os.ReadDir(v.LocalDir)
	if err != nil {
		return 0, err
	}

	versionPattern := regexp.MustCompile(fmt.Sprintf(`%s_(\d+)_`, pattern))
	for _, file := range files {
		matches := versionPattern.FindStringSubmatch(file.Name())
		if len(matches) > 1 {
			var version int
			if _, err := fmt.Sscanf(matches[1], "%d", &version); err == nil {
				return version, nil
			}
		}
	}

	v.AppendOutput(fmt.Sprintf("未找到本地固件版本: %s", filepath.Join(v.LocalDir, pattern)))
	return 0, nil
}

func (v *V3) handleFirmwareVersion(firmware *Firmware) error {
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

	// 重命名文件
	srcFile := filepath.Join(extractDir, firmware.name)
	var destFile string
	switch firmware.name {
	case "cgManager":
		destFile = filepath.Join(v.LocalDir, fmt.Sprintf("cgManager_main_%d_app", firmware.remoteVersion))
	default:
		destFile = filepath.Join(v.LocalDir, fmt.Sprintf("%s_%d_parent", firmware.name, firmware.remoteVersion))
	}

	if err := os.Rename(srcFile, destFile); err != nil {
		return fmt.Errorf("重命名`%s`失败: %v", firmware.name, err)
	}

	// 删除本地旧固件
	switch firmware.name {
	case "cgManager":
		if err := os.RemoveAll(filepath.Join(v.LocalDir, fmt.Sprintf("cgManager_main_%d_app", firmware.localVersion))); err != nil {
			return fmt.Errorf("删除旧固件失败: %v", err)
		}
	default:
		if err := os.RemoveAll(filepath.Join(v.LocalDir, fmt.Sprintf("%s_%d_parent", firmware.name, firmware.localVersion))); err != nil {
			return fmt.Errorf("删除旧固件失败: %v", err)
		}
	}

	v.AppendOutput(fmt.Sprintf("`%s`升级成功!", firmware.name))

	return nil
}

// PackageFirmware 打包固件
func (v *V3) packageFirmware(needRePackage bool) (err error) {
	v.AppendOutput("开始打包固件...")
	tarFile := "v3_init.tar.gz"

	if needRePackage {
		os.Remove(tarFile)
		v.AppendOutput("删除旧固件重新打包...")
	}

	// 检查固件是否已经存在
	if _, err := os.Stat(tarFile); err == nil {
		v.AppendOutput("固件已存在，无需再次打包!")
		time.Sleep(3 * time.Second)
		return nil
	}

	// 打包文件
	dirs := []string{
		"v3_install",
		"share",
	}

	if err = utils.CreateTarGz(tarFile, dirs); err != nil {
		return err
	}

	return nil
}

func (v *V3) DownloadConfig(sn string) error {
	url := fmt.Sprintf("%s/%s", v.InitConfigUrl, sn)

	v.AppendOutput(fmt.Sprintf("开始下载配置文件，请求URL: %s", url))
	response, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("响应码：%v", response.StatusCode)
	}

	var httpResponse struct {
		Success int    `json:"success"`
		Msg     string `json:"msg"`
		Data    string `json:"data"`
		Encrypt string `json:"encrypt"`
	}
	data, err := io.ReadAll(response.Body)
	err = json.Unmarshal(data, &httpResponse)
	if err != nil {
		return err
	}

	if httpResponse.Success != 1 {
		return fmt.Errorf("%v", httpResponse.Msg)
	}

	// 数据解密
	if len(httpResponse.Encrypt) > 0 {
		decrypt, err := utils.DecryptAES256(httpResponse.Data, fmt.Sprintf("%s2cifang", sn), httpResponse.Encrypt)
		if err != nil {
			return err
		}
		httpResponse.Data = decrypt
	}

	_ = os.MkdirAll("setting/v3", 0755)
	err = utils.WriteFile(fmt.Sprintf("setting/v3/%s.json", sn), []byte(httpResponse.Data))

	var info struct {
		Erp struct {
			MqttIp   string `json:"mqtt_ip"`
			MqttPort int    `json:"mqtt_port"`
			MqttUser string `json:"mqtt_user"`
			MqttPwd  string `json:"mqtt_pwd"`
			Tls      int    `json:"tls"`
			Ca       string `json:"ca"`
			Encrypt  int    `json:"encrypt"`
		} `json:"erp"`
		Pro struct {
			Url      string `json:"url"`
			MqttIp   string `json:"mqtt_ip"`
			MqttPort int    `json:"mqtt_port"`
			MqttUser string `json:"mqtt_user"`
			MqttPwd  string `json:"mqtt_pwd"`
			Tls      int    `json:"tls"`
			Ca       string `json:"ca"`
		} `json:"pro"`
		App []string `json:"app"`
	}

	gconv.Struct(httpResponse.Data, &info)
	jsonContent, err := json.Marshal(info)
	v.AppendOutput(string(jsonContent))

	return err

}

// FlashFirmware 刷写固件
func (v *V3) FlashFirmware(devSN string, rePackage bool) {
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

		v.AppendOutput("开始刷写v3程序，执行过程请勿关闭程序!")

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
			if err = v.UploadFile("v3_init.tar.gz", "/tmpcf/v3_init.tar.gz"); err != nil {
				return
			}

			if err = v.UploadFile("v3_install.sh", "/tmpcf/v3_install.sh"); err != nil {
				return
			}

			// 执行脚本
			if _, err = v.RunAndWaitCommand(fmt.Sprintf("cd /tmpcf && sh v3_install.sh %s", devSN)); err != nil {
				return
			}

			// 尝试将初始配置文件传到设备
			if err = v.UploadFile(fmt.Sprintf("setting/v3/%s.json", devSN), "/datas/cf_go_v3/data/cache/settings.json"); err == nil {
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
