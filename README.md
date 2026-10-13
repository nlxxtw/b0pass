# NextPass

局域网作业助手：老师发资料 / 收作业，学生只需打开根地址。

基于 [b0pass](https://github.com/nearthink/b0pass) 改造。

## 使用

1. 下载 Release 中的 `nextpass-win.zip`（或 linux/mac）
2. 解压运行 `nextpass.exe`
3. **学生访问**：`http://老师电脑IP:8000/`
4. **老师管理**：`http://老师电脑IP:8000/admin`  
   默认管理口令：`admin`（可在同目录 `config.ini` 修改 `CodeReadWrite`）

## 功能

- 发资料 / 收作业两种活动
- 老师可勾选：强制学号姓名、学生是否可见/下载他人作业
- 提交统计（人数、文件、时间）
- 投屏只需根链接，无需额外路径

## 开发

```bash
cd main
go run .
```

GitHub Actions 在推送 `v*` 标签时自动编译发布。
