package tool

import (
	"fyne.io/fyne/v2/canvas"
	"image/color"
	"net"
	"time"
)

// RWTimeoutConn 封装了一个具有读写超时的连接
type RWTimeoutConn struct {
	net.Conn                   // 原始的连接
	ReadTimeout  time.Duration // 读超时
	WriteTimeout time.Duration // 写超时
}

func (c *RWTimeoutConn) Read(b []byte) (int, error) {
	err := c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))
	if err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *RWTimeoutConn) Write(b []byte) (int, error) {
	err := c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
	if err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}

// ConnStatusDisplay 封装了一个用于显示连接状态的控件
type ConnStatusDisplay struct {
	canvas.Text
}

func NewConnStatusDisplay() *ConnStatusDisplay {
	sd := &ConnStatusDisplay{}
	sd.Text.TextSize = 12
	sd.SetStatus(false)
	return sd
}
func (sd *ConnStatusDisplay) SetStatus(connected bool) {
	if connected {
		sd.Text.Text = "已连接"
		sd.Text.Color = color.RGBA{G: 255, A: 255} // 绿色
	} else {
		sd.Text.Text = "未连接"
		sd.Text.Color = color.RGBA{R: 255, A: 255} // 红色
	}
	sd.Text.Refresh()
}
