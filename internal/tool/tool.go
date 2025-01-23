package tool

import (
	"EMInit/internal/version"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/flopp/go-findfont"
	"github.com/tmc/scp"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const USERNAME = "root"
const PASSWORD = "root"

func init() {
	// 查找系统中的中文字体文件路径
	fontPaths := findfont.List()

	preferredFonts := []string{"simkai.ttf", "msyh.ttf"}
	for _, font := range preferredFonts {
		for _, path := range fontPaths {
			if strings.Contains(path, font) {
				os.Setenv("FYNE_FONT", path)
				break
			}
		}
	}
}

type FirmwareFlashTool struct {
	// 用于ssh连接
	sshClient *ssh.Client
	sshCancel context.CancelFunc
	lock      sync.Mutex

	syncTime     bool  // 是否同步时间
	updateStatus int32 // 更新状态

	// 用于管理命令执行
	cmdCancel  context.CancelFunc
	cmdLock    sync.Mutex
	cmdRunning bool

	version version.IFirmwareVersion

	// 定义UI组件
	app                fyne.App
	window             fyne.Window       // 主窗口
	output             *widget.Entry     // 输出框
	outputScroll       *container.Scroll // 输出框
	ipEntry            *widget.Entry     // IP
	syncTimeCheck      *widget.Check     // 同步时间
	snEntry            *widget.Entry     // SN输入框
	downloadButton     *widget.Button    // 下载初始配置按钮
	versionSelect      *widget.Select    // 版本
	connButton         *widget.Button    // 连接按钮
	flashButton        *widget.Button    // 刷写按钮
	updateButton       *widget.Button    // 检查更新按钮
	net1Select         *widget.Select    // NET1选择框
	net1AddressEntry   *widget.Entry     // NET1地址输入框
	net1NetmaskEntry   *widget.Entry     // NET1子网掩码输入框
	net1GatewayEntry   *widget.Entry     // NET1网关输入框
	net2Select         *widget.Select    // NET2选择框
	net2AddressEntry   *widget.Entry     // NET2地址输入框
	net2NetmaskEntry   *widget.Entry     // NET2子网掩码输入框
	net2GatewayEntry   *widget.Entry     // NET2网关输入框
	helpLabel          *widget.Label
	helpScroll         *container.Scroll
	*ConnStatusDisplay // 用于显示SSH连接状态
}

func NewFirmwareFlashTool() *FirmwareFlashTool {
	return &FirmwareFlashTool{
		app:               app.New(),
		window:            nil,
		output:            widget.NewMultiLineEntry(),
		ipEntry:           widget.NewEntry(),
		snEntry:           widget.NewEntry(),
		net1AddressEntry:  widget.NewEntry(),
		net1NetmaskEntry:  widget.NewEntry(),
		net1GatewayEntry:  widget.NewEntry(),
		net2AddressEntry:  widget.NewEntry(),
		net2NetmaskEntry:  widget.NewEntry(),
		net2GatewayEntry:  widget.NewEntry(),
		ConnStatusDisplay: NewConnStatusDisplay(),
		net1Select:        widget.NewSelect([]string{"WAN", "LAN", ""}, nil),
		net2Select:        widget.NewSelect([]string{"WAN", "LAN", ""}, nil),
	}
}
func (t *FirmwareFlashTool) Run() {
	t.window = t.app.NewWindow("EM500 初始化工具v1.1")
	t.setupUI()
	t.window.ShowAndRun()
}

func (t *FirmwareFlashTool) setupUI() {
	t.setupEntries()
	t.setupPlaceholders()
	t.setupSelects()
	t.setupButtons()

	tabs := t.setupTabs()
	t.window.SetContent(tabs)
	t.preloadTabs(tabs)
	t.window.SetFixedSize(true)
	t.window.Resize(fyne.NewSize(600, 640))
}

func (t *FirmwareFlashTool) setupEntries() {
	t.ipEntry.SetText("192.168.2.136")
	t.snEntry.SetText("")
}

func (t *FirmwareFlashTool) setupPlaceholders() {
	t.output.SetPlaceHolder("输出...")
	t.snEntry.SetPlaceHolder("请输入设备SN")
	t.net1AddressEntry.SetPlaceHolder("输入 LAN1 IP 地址，不可为空")
	t.net1NetmaskEntry.SetPlaceHolder("输入 LAN1 子网掩码，不可为空")
	t.net1GatewayEntry.SetPlaceHolder("输入 LAN1 网关地址，可为空")
	t.net2AddressEntry.SetPlaceHolder("输入 LAN2 IP 地址，不可为空")
	t.net2NetmaskEntry.SetPlaceHolder("输入 LAN2 子网掩码，不可为空")
	t.net2GatewayEntry.SetPlaceHolder("输入 LAN2 网关地址，可为空")
}

func (t *FirmwareFlashTool) setupSelects() {
	versions := []string{"v2", "v3"}

	t.versionSelect = widget.NewSelect(versions, func(value string) {
		switch value {
		case "v2":
			t.AppendOutput("切换到v2版本")
			t.version = version.NewV2(t, t.window)
		case "v3":
			t.AppendOutput("切换到v3版本")
			t.version = version.NewV3(t, t.window)
		}
	})
	t.versionSelect.Selected = "v3"
	t.version = version.NewV3(t, t.window)

	hideLAN1Input := func() {
		t.net1AddressEntry.Hide()
		t.net1NetmaskEntry.Hide()
		t.net1GatewayEntry.Hide()
	}
	showLAN1Input := func() {
		t.net1AddressEntry.Show()
		t.net1NetmaskEntry.Show()
		t.net1GatewayEntry.Show()
	}
	hideLAN2Input := func() {
		t.net2AddressEntry.Hide()
		t.net2NetmaskEntry.Hide()
		t.net2GatewayEntry.Hide()
	}
	showLAN2Input := func() {
		t.net2AddressEntry.Show()
		t.net2NetmaskEntry.Show()
		t.net2GatewayEntry.Show()
	}

	t.net1Select.OnChanged = func(selected string) {
		if selected == "LAN" {
			showLAN1Input()
		} else {
			hideLAN1Input()
		}
	}

	t.net2Select.OnChanged = func(selected string) {
		if selected == "LAN" {
			showLAN2Input()
		} else {
			hideLAN2Input()
		}
	}

	// 默认隐藏 LAN 输入框
	hideLAN1Input()
	hideLAN2Input()
}

func (t *FirmwareFlashTool) setupButtons() {
	t.output.Wrapping = fyne.TextWrapWord
	t.outputScroll = container.NewVScroll(t.output)
	t.outputScroll.SetMinSize(fyne.NewSize(460, 600))

	t.downloadButton = widget.NewButton("下载初始配置", func() {
		if t.snEntry == nil || t.snEntry.Text == "" {
			dialog.ShowInformation("错误", "请输入设备SN", t.window)
			return
		}

		sn := t.snEntry.Text
		if err := t.version.DownloadConfig(sn); err != nil {
			t.AppendOutput(fmt.Sprintf("下载配置失败! 设备SN: %s, 错误信息: %s", sn, err.Error()))
		} else {
			t.AppendOutput(fmt.Sprintf("下载配置成功! 设备SN: %s", sn))
		}
	})

	t.connButton = widget.NewButton("建立连接", func() {
		go func() {
			err := t.connToDevice()
			if err != nil {
				t.AppendOutput("连接失败: " + err.Error())
			} else {
				t.AppendOutput("连接已建立!")
			}
		}()
	})

	t.flashButton = widget.NewButton("开始刷写", func() {
		t.version.FlashFirmware(t.snEntry.Text, true)
	})

	t.updateButton = widget.NewButton("检查更新", func() {
		go func() {
			t.AppendOutput("开始检查固件OTA版本，执行过程请勿关闭程序!")
			if err := t.version.CheckFirmwareVersion(); err != nil {
				t.AppendOutput("更新固件失败: " + err.Error())
			}
		}()
	})
}

func (t *FirmwareFlashTool) setupTabs() *container.AppTabs {
	t.syncTimeCheck = widget.NewCheck("同步主机时间", func(check bool) {
		t.syncTime = check
	})
	t.syncTimeCheck.SetChecked(true)

	ipBox := container.NewVBox(
		container.NewHBox(widget.NewLabel("当前连接状态:"), &t.ConnStatusDisplay.Text),
		widget.NewLabel("目标设备IP:"),
		t.ipEntry,
		t.connButton,
	)

	outputBox := container.NewVBox(
		widget.NewLabel("输出:"),
		t.outputScroll,
		container.NewHBox(layout.NewSpacer(), widget.NewButton("清空内容", func() {
			t.output.SetText("")
		})),
	)

	tab1Content := container.NewHSplit(
		container.NewVBox(
			container.NewVBox(widget.NewLabel("选择版本:"), t.versionSelect),
			t.updateButton,
			container.NewVBox(widget.NewLabel("目标设备SN:"), t.snEntry),
			t.downloadButton,
		),
		outputBox,
	)

	tab2Content := container.NewHSplit(
		container.NewVBox(
			ipBox,
			container.NewVBox(widget.NewLabel("目标设备SN:"), t.snEntry),
			container.NewVBox(widget.NewLabel("选择版本:"), t.versionSelect),
			t.flashButton,
		),
		outputBox,
	)

	tab3Content := container.NewHSplit(
		container.NewVBox(
			ipBox,
			container.NewHBox(t.syncTimeCheck),
			widget.NewLabel("选择 NET1 网口配置："),
			t.net1Select,
			t.net1AddressEntry,
			t.net1NetmaskEntry,
			t.net1GatewayEntry,
			widget.NewLabel("选择 NET2 网口配置："),
			t.net2Select,
			t.net2AddressEntry,
			t.net2NetmaskEntry,
			t.net2GatewayEntry,
			widget.NewButton("更新设置", func() {
				t.updateSetting()
			}),
		),
		outputBox,
	)

	content := widget.NewLabel(helpContent)
	content.Wrapping = fyne.TextWrapWord
	t.helpScroll = container.NewScroll(content)
	t.helpScroll.SetMinSize(fyne.NewSize(600, 640))

	return container.NewAppTabs(
		container.NewTabItem("更新管理", tab1Content),
		container.NewTabItem("固件刷写", tab2Content),
		container.NewTabItem("设备管理", tab3Content),
		container.NewTabItem("帮助文档", container.NewVBox(
			t.helpScroll,
		)),
	)
}

func (t *FirmwareFlashTool) preloadTabs(tabs *container.AppTabs) {
	// 提前加载标签页内容
	tabs.SelectIndex(2)
	tabs.SelectIndex(1)
	tabs.SelectIndex(0)
}

// connToDevice 建立SSH连接
func (t *FirmwareFlashTool) connToDevice() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	// 关闭旧的连接
	if t.sshClient != nil {
		t.sshClient.Close()
		t.sshClient = nil
		t.ConnStatusDisplay.SetStatus(false)

		if t.sshCancel != nil {
			t.sshCancel()
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.AppendOutput(fmt.Sprintf("正在建立与设备（IP：`%s`）的SSH连接", t.ipEntry.Text))
	sshConfig := &ssh.ClientConfig{
		User: USERNAME,
		Auth: []ssh.AuthMethod{
			ssh.Password(PASSWORD),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:22", t.ipEntry.Text)

	// 设置连接超时时间
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}

	// 设置读写超时时间
	timeoutConn := &RWTimeoutConn{conn, 5 * time.Second, 5 * time.Second}
	sshConn, chans, reqs, err := ssh.NewClientConn(timeoutConn, addr, sshConfig)
	if err != nil {
		return err
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	t.sshClient = client
	t.ConnStatusDisplay.SetStatus(true) // 设置连接状态为已连接

	ctx, cancel := context.WithCancel(context.Background())
	t.sshCancel = cancel

	// 启动监控连接
	go t.monitorConnection(ctx)

	return nil
}

func (t *FirmwareFlashTool) monitorConnection(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.AppendOutput("旧连接已断开!")
			return
		case <-ticker.C:
			if !t.keepalive() {
				return
			}
		}
	}
}

func (t *FirmwareFlashTool) keepalive() bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.sshClient != nil {
		// 发送心跳包，检测连接是否正常
		_, _, err := t.sshClient.Conn.SendRequest("keepalive@golang.org", true, nil)
		if err != nil {
			t.sshClient.Close()
			t.sshClient = nil

			t.AppendOutput("检测到连接已断开!")
			t.ConnStatusDisplay.SetStatus(false)

			return false
		}
	}

	return true
}

func (t *FirmwareFlashTool) updateSetting() {
	if !atomic.CompareAndSwapInt32(&t.updateStatus, 0, 1) {
		t.AppendOutput("正在更新设置!")
		dialog.ShowInformation("警告", "正在更新设置!", t.window)
		return
	}

	conf := dialog.NewConfirm("更新设置", "您确定要更新设置吗？", func(confirmed bool) {
		if !confirmed {
			atomic.StoreInt32(&t.updateStatus, 0)
			return
		}

		go func() {
			var err error

			defer func() {
				atomic.StoreInt32(&t.updateStatus, 0)
				if err != nil {
					t.AppendOutput("更新设置失败: " + err.Error())
				} else {
					t.AppendOutput("更新设置成功！")
				}
			}()

			var cmd string
			var reboot bool

			fmt.Println(t.net1Select.Selected)

			// 同步时间
			if t.syncTime {
				t.AppendOutput("正在同步当前主机时间...")
				dateTime := time.Now()
				cmd = fmt.Sprintf("date -s \"%s\"", dateTime.Format("2006-01-02 15:04:05"))
				if _, err = t.RunAndWaitCommand(cmd); err != nil {
					return
				}

				// 写入硬件时钟
				cmd = fmt.Sprintf("hwclock -w")
				if _, err = t.RunAndWaitCommand(cmd); err != nil {
					return
				}
			}

			// 修改网络配置
			net1Type := t.net1Select.Selected
			net2Type := t.net2Select.Selected
			if (net1Type != "" && net2Type == "") || (net1Type == "" && net2Type != "") {
				err = fmt.Errorf("两个网口必须一起配置！")
				return
			} else if net1Type != "" && net2Type != "" {
				if (net1Type == "LAN" && (t.net1AddressEntry.Text == "" || t.net1NetmaskEntry.Text == "")) ||
					(net2Type == "LAN" && (t.net2AddressEntry.Text == "" || t.net2NetmaskEntry.Text == "")) {
					err = fmt.Errorf("请确保两个网口都已配置并填写完整！")
					return
				}

				var netConfig []string
				netConfig = append(netConfig, generateNetworkConfig("eth0", net1Type, t.net1AddressEntry.Text, t.net1NetmaskEntry.Text, t.net1GatewayEntry.Text))
				netConfig = append(netConfig, generateNetworkConfig("eth1", net2Type, t.net2AddressEntry.Text, t.net2NetmaskEntry.Text, t.net2GatewayEntry.Text))

				// 备份原始配置
				if _, err = t.RunAndWaitCommand("cp /etc/network/interfaces /etc/network/interfaces.bak.$(date +%Y%m%d%H%M%S)"); err != nil {
					return
				}
				// 写入配置文件
				if _, err = t.RunAndWaitCommand(fmt.Sprintf("echo \"%s\" > /etc/network/interfaces", strings.Join(netConfig, "\n\n"))); err != nil {
					return
				}
				reboot = true
			}

			if reboot {
				t.AppendOutput("系统设置更新成功，正在重启设备...")
				if _, err = t.RunAndWaitCommand("reboot"); err != nil {
					return
				}
			}
		}()

	}, t.window)

	conf.SetConfirmText("是")
	conf.SetDismissText("否")
	conf.Show()
}

func generateNetworkConfig(interfaceName, netType, address, netmask, gateway string) string {
	if netType == "WAN" {
		return fmt.Sprintf("auto %s\niface %s inet dhcp", interfaceName, interfaceName)
	} else {
		content := fmt.Sprintf("auto %s\niface %s inet static\naddress %s\nnetmask %s", interfaceName, interfaceName, address, netmask)
		if gateway != "" {
			return fmt.Sprintf("%s\ngateway %s", content, gateway)
		}

		return content
	}
}

func (t *FirmwareFlashTool) RunAndWaitCommand(cmd string) (string, error) {
	return t.runCommand(cmd)
}

// runCommand 执行命令并输出结果
func (t *FirmwareFlashTool) runCommand(cmd string) (string, error) {
	t.cmdLock.Lock()
	defer t.cmdLock.Unlock()

	if t.sshClient == nil {
		return "", errors.New("未连接到设备，请先与设备建立连接")
	}

	if t.cmdRunning {
		return "", fmt.Errorf("正在执行命令中。。")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cmdCancel = cancel
	t.cmdRunning = true
	defer func() {
		t.cmdRunning = false
	}()

	outputBuf := new(bytes.Buffer)
	outputCh := make(chan string)
	errCh := make(chan error)

	go func() {
		session, err := t.sshClient.NewSession()
		if err != nil {
			errCh <- err
			return
		}
		defer session.Close()

		// 获取标准输出和标准错误管道
		stdout, err := session.StdoutPipe()
		if err != nil {
			errCh <- err
			return
		}
		stderr, err := session.StderrPipe()
		if err != nil {
			errCh <- err
			return
		}

		go t.printOutput(stdout, outputBuf)
		go t.printOutput(stderr, outputBuf)

		errChRun := make(chan error, 1)
		go func() {
			t.AppendOutput("执行命令: " + cmd)
			errChRun <- session.Run(cmd)
		}()

		select {
		case <-ctx.Done():
			session.Close()
			<-errChRun
			errCh <- errors.New("命令终止")
		case err = <-errChRun:
			if err != nil {
				errCh <- err
				return
			}
			outputCh <- outputBuf.String()
		}
	}()

	select {
	case err := <-errCh:
		return "", err
	case output := <-outputCh:
		return output, nil
	}
}

// printOutput 读取并打印 SSH 输出
func (t *FirmwareFlashTool) printOutput(reader io.Reader, outputBuf *bytes.Buffer) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		t.AppendOutput(line)
		outputBuf.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		t.AppendOutput("Error reading output: " + err.Error())
	}
}

func (t *FirmwareFlashTool) AppendOutput(message string) {
	timestamp := time.Now().Format("15:04:05")
	t.output.Append(timestamp + " " + message + "\n")

	t.outputScroll.ScrollToBottom()
}

func (t *FirmwareFlashTool) UploadFile(localPath, remotePath string) error {
	session, err := t.sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if err := scp.CopyPath(localPath, remotePath, session); err != nil {
		return err
	}

	t.AppendOutput(fmt.Sprintf("文件上传成功: %s -> %s", localPath, remotePath))
	return err
}
