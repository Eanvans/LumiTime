# Subtuber Services 部署包

## 部署步骤

### 1. 上传文件到Ubuntu服务器
```bash
# 在本地执行
scp -r release/* user@your-ubuntu-server:/path/to/deployment/
```

### 2. 在Ubuntu服务器上设置
```bash
# 进入部署目录
cd /path/to/deployment/

# 复制并编辑配置文件
cp config.yaml.example config.yaml
nano config.yaml  # 编辑配置信息

# 确保二进制文件可执行
chmod +x subtuber_services
```

### 3. 运行服务

#### 方式1: 直接运行
```bash
./subtuber_services
```

#### 方式2: 使用nohup后台运行
```bash
nohup ./subtuber_services > app.log 2>&1 &
```

#### 方式3: 使用systemd服务（推荐）
创建服务文件 `/etc/systemd/system/subtuber.service`:
```ini
[Unit]
Description=Subtuber Services
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/deployment
ExecStart=/path/to/deployment/subtuber_services
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

然后启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable subtuber
sudo systemctl start subtuber
sudo systemctl status subtuber
```

### 4. 查看日志
```bash
# 如果使用nohup
tail -f app.log

# 如果使用systemd
sudo journalctl -u subtuber -f
```

### 5. 防火墙设置（如果需要）
```bash
# 开放8080端口（根据实际端口修改）
sudo ufw allow 8080
```

## 配置说明

请在 `config.yaml` 中配置以下信息：
- SMTP邮件服务器设置
- Twitch API凭证
- Google/Alibaba AI API密钥
- RPC服务地址

## 目录结构
- `App_Data/` - 应用数据
- `downloads/` - 下载文件
- `analysis_results/` - 分析结果
- `chat_logs/` - 聊天日志

## 维护

### 停止服务
```bash
# 如果使用systemd
sudo systemctl stop subtuber

# 如果使用nohup，找到进程并kill
ps aux | grep subtuber_services
kill <PID>
```

### 更新服务
1. 停止服务
2. 备份当前数据
3. 上传新的二进制文件
4. 重启服务
