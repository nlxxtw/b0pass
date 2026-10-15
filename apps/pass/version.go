package app

// AppVersion 当前程序版本（发布时由 CI -ldflags 注入，勿加 v 前缀）
var AppVersion = "2.1.7"

// GithubRepo Releases 仓库
const GithubRepo = "nlxxtw/b0pass"

// GithubMirror 国内加速前缀（拼在原始 https://github.com 或 api 地址前）
const GithubMirror = "https://github.19492035.xyz/"
