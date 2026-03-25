Write-Host "======================================" -ForegroundColor Cyan
Write-Host "欢迎使用 BetterLan 云端节点一键安装程序" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""

$InstallDir = "C:\BetterLanServer"
$DownloadUrl = "https://github.com/KDronin/BetterLAN-server/releases/download/v0.0.1/betterlan-server-windows-amd64.exe" 

$UserIP = Read-Host "1. 请输入节点绑定的 IP 地址 [默认 0.0.0.0]"
if ([string]::IsNullOrWhiteSpace($UserIP)) { $UserIP = "0.0.0.0" }

$UserPort = Read-Host "2. 请输入自定义节点端口 [默认 45678]"
if ([string]::IsNullOrWhiteSpace($UserPort)) { $UserPort = 45678 }

$AutoStartInput = Read-Host "3. 是否设置开机自启动？(Y/n)"
if ([string]::IsNullOrWhiteSpace($AutoStartInput)) { $AutoStartInput = "Y" }
$EnableAutoStart = ($AutoStartInput -match "^[Yy]$")

$StartNowInput = Read-Host "4. 安装完成后是否立即启动？(Y/n)"
if ([string]::IsNullOrWhiteSpace($StartNowInput)) { $StartNowInput = "Y" }
$StartNow = ($StartNowInput -match "^[Yy]$")

Write-Host "`n正在为您执行自动化部署...`n" -ForegroundColor Cyan

if (-Not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}
Set-Location $InstallDir

$Utf8NoBom = New-Object System.Text.UTF8Encoding($False)
$Config = @{
    ip = $UserIP
    port = [int]$UserPort
}
$JsonStr = $Config | ConvertTo-Json
[System.IO.File]::WriteAllText("$InstallDir\config.json", $JsonStr, $Utf8NoBom)
Write-Host "[+] 配置文件 config.json 生成完毕" -ForegroundColor Green
Write-Host "[+] 正在下载服务端程序..." -ForegroundColor Yellow
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile "betterlan-server.exe"
} catch {
    Write-Host "下载失败！请检查网络或通知作者" -ForegroundColor Red
    Read-Host "按回车键退出..."
    exit
}
Write-Host "[+] 核心程序下载完成" -ForegroundColor Green
Write-Host "[+] 正在生成启动脚本..." -ForegroundColor Yellow
$BatLines = @(
    "@echo off"
    "chcp 65001 >nul"
    "title BetterLan Relay Server (运行中)"
    "color 0A"
    "cd /d ""%~dp0"""
    "echo =========================================="
    "echo BetterLan 云端节点已启动！请勿关闭此窗口。"
    "echo =========================================="
    "betterlan-server.exe"
    "pause"
)
$BatContent = $BatLines -join "`r`n"
[System.IO.File]::WriteAllText("$InstallDir\启动节点.bat", $BatContent, $Utf8NoBom)
Write-Host "[+] 启动脚本 [启动节点.bat] 生成完毕" -ForegroundColor Green

$RegPath = "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run"
$RegName = "BetterLanRelay"

if ($EnableAutoStart) {
    Write-Host "[+] 正在配置开机自启..." -ForegroundColor Yellow
    Set-ItemProperty -Path $RegPath -Name $RegName -Value "`"$InstallDir\启动节点.bat`""
    Write-Host "[+] 开机自启已开启！每次开机将自动弹出运行窗口。" -ForegroundColor Green
} else {
    Remove-ItemProperty -Path $RegPath -Name $RegName -ErrorAction SilentlyContinue
    Write-Host "[-] 已跳过开机自启配置。" -ForegroundColor Gray
}

if ($StartNow) {
    Write-Host "[+] 正在为您拉起服务端进程..." -ForegroundColor Yellow
    Stop-Process -Name "betterlan-server" -ErrorAction SilentlyContinue
    Start-Process -FilePath "$InstallDir\启动节点.bat"
    Write-Host "[+] 服务端窗口已弹出！" -ForegroundColor Green
} else {
    Write-Host "[-] 已跳过立即启动。请日后手动双击运行。" -ForegroundColor Gray
}

Write-Host "`n==========================================" -ForegroundColor Cyan
Write-Host "节点部署完成！" -ForegroundColor Green
Write-Host "程序目录: $InstallDir"
Write-Host "如需手动启动或重启，请双击目录下的【启动节点.bat】。"
Write-Host "==========================================" -ForegroundColor Cyan
Read-Host "按回车键退出安装程序..."
