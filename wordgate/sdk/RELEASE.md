# 发布新版本

本文档描述了如何发布WordGate SDK的新版本。

## 准备工作

1. 确保所有更改已提交并推送到主分支。
2. 确保代码已经过测试并且可以正常工作。
3. 确保GitHub仓库中已创建`apt-repository`分支，用于存储APT仓库数据。

## 发布步骤

1. 更新版本号并提交更改：

```bash
# 提交所有更改
git add .
git commit -m "准备发布版本 X.Y.Z"
git push origin main
```

2. 创建并推送新标签：

```bash
# 使用语义化版本号创建标签
git tag -a vX.Y.Z -m "版本 X.Y.Z"

# 推送标签到GitHub
git push origin vX.Y.Z
```

3. 监控GitHub Actions工作流程：

一旦推送了新标签，GitHub Actions工作流程将自动启动，构建应用程序并创建发布。
您可以在GitHub仓库的"Actions"选项卡中监控工作流程的进度。

4. 验证发布：

工作流程完成后，验证以下内容：

- GitHub Releases页面上是否有新的发布
- 发布包含Linux和Mac的二进制文件
- Debian包(.deb)文件是否包含在发布中
- Homebrew Formula是否创建并包含在发布中
- APT仓库是否更新

## 安装验证

发布后，验证安装流程：

### Mac验证

```bash
brew tap allnationconnect/wordgate
brew install wordgate
wordgate --version
```

### Linux验证

```bash
echo "deb [trusted=yes] https://raw.githubusercontent.com/allnationconnect/sdk/apt-repository/apt-repo/ /" | sudo tee /etc/apt/sources.list.d/wordgate.list
sudo apt-get update
sudo apt-get install wordgate
wordgate --version
``` 