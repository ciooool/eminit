#!/bin/bash

set -e  # 脚本在遇到任何错误时退出

# 日志函数
log() {
    echo "$1"
}

# 错误处理函数
error_exit() {
    log "ERROR: $1"
    exit 1
}

if [ $# -ne 1 ]; then
    error_exit "请输入网关SN号"
fi

gwSN=$1

log "开始执行初始化脚本..."

PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin:~/bin
export PATH
LANG=en_US.UTF-8

# 临时目录
tmp_path="/tmpcf"
tmp_share_path="${tmp_path}/share"
tmp_v3_path="${tmp_path}/v3_install"
# 压缩包
tar_file="v3_init.tar.gz"

# 程序主目录
root_path="/datas"
# 项目主目录
pro_dir="cf_go_v3"
# frp主目录
frpc_dir="frpc"

if [ ! -d $tmp_path ]; then
    error_exit "程序请放在$tmp_path目录下"
else
    if [ ! -e "$tmp_path/$tar_file" ]; then
        error_exit "请先将$tar_file文件放在$tmp_path目录下"
    else
        cd $tmp_path || error_exit "无法进入目录 $tmp_path"
        
        tar -zxvf $tar_file || error_exit "解压 $tar_file 失败"

        # 删除旧的文件
        rm -rf $root_path/$pro_dir/* || error_exit "删除旧的 $root_path/$pro_dir 目录文件失败"
        rm -rf $root_path/$frpc_dir/* || error_exit "删除旧的 $root_path/$frpc_dir 目录文件失败"

        mkdir -p $root_path/$pro_dir/bin
        mkdir -p $root_path/$pro_dir/data/config
        mkdir -p $root_path/$pro_dir/data/cache
        mkdir -p $root_path/$frpc_dir

        mv $tmp_share_path/frpc/frpc $root_path/$frpc_dir/frpc || error_exit "移动 frpc 文件失败"
        mv $tmp_share_path/frpc/frpc.toml $root_path/$frpc_dir/frpc.toml || error_exit "移动 frpc.toml 文件失败"
        mv $tmp_v3_path/bin/* $root_path/$pro_dir/bin || error_exit "移动 bin 文件失败"
        mv $tmp_v3_path/config/* $root_path/$pro_dir/data/config || error_exit "移动 config 文件失败"

        # 修改cgManager.yaml的网关型号
        sed -i "s/  device_type: \".*\"/  device_type: \"EM500\"/" $root_path/$pro_dir/data/config/cgManager.yaml || error_exit "修改 cgManager.yaml 文件失败"
        sed -i "s/  device_sn: \".*\"/  device_sn: \"${gwSN}\"/" $root_path/$pro_dir/data/config/cgManager.yaml || error_exit "修改 cgManager.yaml 文件失败"

        chmod -R 755 $root_path/$pro_dir/bin
        chmod -R 755 $root_path/$frpc_dir

        # 修改frpc.toml的name
        sed -i "s/name = \".*\"/name = \"${gwSN}\"/" $root_path/$frpc_dir/frpc.toml || error_exit "修改 frpc.toml 文件失败"

        # 修改时区
        sudo timedatectl set-timezone Asia/Shanghai || error_exit "设置时区失败"

        # 如果系统服务已经存在，先停止并禁用它们
        systemctl stop frpc || true
        systemctl stop cgKeepalive || true
        systemctl stop cgCollector || true
        systemctl stop cgUpdater || true

        systemctl disable frpc || true
        systemctl disable cgKeepalive || true
        systemctl disable cgCollector || true
        systemctl disable cgUpdater || true

        echo "[Unit]
Description = cgKeepalive
After = syslog.target

[Service]
Type = simple
ExecStartPre = /bin/sleep 5
WorkingDirectory = $root_path/$pro_dir/bin/
ExecStart = $root_path/$pro_dir/bin/cgKeepalive
Restart = always
RestartSec = 5

[Install]
WantedBy = multi-user.target" | sudo tee /etc/systemd/system/cgKeepalive.service > /dev/null

        echo "[Unit]
Description = frpc
After = syslog.target

[Service]
Type = simple
ExecStartPre = /bin/sleep 5
ExecStart = $root_path/$frpc_dir/frpc -c $root_path/$frpc_dir/frpc.toml
Restart = always
RestartSec = 5

[Install]
WantedBy = multi-user.target" | sudo tee /etc/systemd/system/frpc.service > /dev/null

        chmod 644 /etc/systemd/system/cgKeepalive.service
        chmod 644 /etc/systemd/system/frpc.service

        systemctl daemon-reload || error_exit "重新加载 systemd 守护进程失败"
        systemctl enable frpc || error_exit "启用 frpc 服务失败"
        systemctl enable cgKeepalive || error_exit "启用 cgKeepalive 服务失败"

        systemctl start frpc || error_exit "启动 frpc 服务失败"
        systemctl start cgKeepalive || error_exit "启动 cgKeepalive 服务失败"

        cd $tmp_share_path/bsdiff/ || error_exit "无法进入目录 $tmp_share_path/bsdiff/"
        dpkg -i bsdiff_4.3-21_armhf.deb || error_exit "安装 bsdiff_4.3-21_armhf.deb 失败"
        apt-get install -f || true

        systemctl disable networking.service || error_exit "禁用开机自启动networking失败"
        if [ -e /usr/bin/em500-test.sh ]; then
            echo "systemctl start networking" >> /usr/bin/em500-test.sh || error_exit "修改 em500-test.sh 文件失败"
        else
            echo "systemctl start networking" >> /usr/bin/zlg-test.sh || error_exit "修改 zlg-test.sh 文件失败"
        fi

        log "初始化成功..."
        ps -A | grep cg
    fi
fi

exit 0
