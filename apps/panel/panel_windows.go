//go:build windows

package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	passapp "b0go/apps/pass"
	"b0go/apps/pass/lib/folderdialog"
	"b0go/core/applog"
	"b0go/core/gateway"
	"b0go/core/tools/cmd"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func panelErr(msg string) {
	_ = os.MkdirAll(filepath.Join("files", "_nextpass"), 0755)
	_ = os.WriteFile(filepath.Join("files", "_nextpass", "panel_error.txt"), []byte(time.Now().Format("2006-01-02 15:04:05")+" "+msg+"\n"), 0644)
	walk.MsgBox(nil, "NextPass", msg, walk.MsgBoxIconError)
}

func resolveLogPath() string {
	logPath := filepath.Join("files", "_nextpass", "app.log")
	cfg := loadPanelConfig()
	if p := strings.TrimSpace(cfg.Path); p != "" {
		logPath = filepath.Join(p, "_nextpass", "app.log")
	}
	return logPath
}

// RunDesktop 原生面板。成功返回 true。
func RunDesktop() bool {
	_ = os.MkdirAll("files", 0666)
	if _, err := os.Stat(configFileName); err != nil {
		_ = savePanelConfig(defaultPanelConfig())
	}
	// 尽早挂上文件日志，面板「运行日志」里马上能看到
	_ = applog.Setup(resolveLogPath())
	applog.Info("面板已打开")

	var (
		mw                                      *walk.MainWindow
		pathEdit, portEdit, pwdEdit             *walk.LineEdit
		autoOpenCB                              *walk.CheckBox
		statusLabel, linkLabel, verLabel, tipLB *walk.Label
		logEdit                                 *walk.TextEdit
		btnStart, btnStop, btnAdmin, btnStudent *walk.PushButton
	)

	readForm := func() PanelConfig {
		return PanelConfig{
			Port:     strings.TrimSpace(portEdit.Text()),
			Path:     strings.TrimSpace(pathEdit.Text()),
			Password: pwdEdit.Text(),
			AutoOpen: autoOpenCB.Checked(),
		}
	}

	applyState := func(st State) {
		if statusLabel == nil {
			return
		}
		_ = verLabel.SetText("v" + st.Version)
		if portEdit != nil && strings.TrimSpace(st.Port) != "" {
			_ = portEdit.SetText(st.Port)
		}
		on := st.Running
		if on {
			_ = statusLabel.SetText("服务运行中")
			line := "学生: " + st.StudentURL + "\r\n管理: " + st.AdminURL
			if strings.Contains(st.Message, "已自动改用") {
				line = st.Message + "\r\n" + line
			}
			_ = linkLabel.SetText(line)
		} else {
			_ = statusLabel.SetText("服务未启动")
			msg := st.Message
			if msg == "" {
				msg = "点击「启动服务」后，学生用浏览器访问"
			}
			_ = linkLabel.SetText(msg)
		}
		if forceUpdateLocked {
			btnStart.SetEnabled(false)
			btnStop.SetEnabled(false)
			btnAdmin.SetEnabled(false)
			btnStudent.SetEnabled(false)
		} else {
			btnStart.SetEnabled(!on)
			btnStop.SetEnabled(on)
			btnAdmin.SetEnabled(on)
			btnStudent.SetEnabled(on)
		}
	}

	syncApply := func(st State) {
		if mw == nil {
			return
		}
		mw.Synchronize(func() { applyState(st) })
	}

	err := MainWindow{
		AssignTo: &mw,
		Title:    "NextPass",
		MinSize:  Size{Width: 440, Height: 520},
		Size:     Size{Width: 460, Height: 580},
		Layout:   VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "NextPass 教师机", Font: Font{PointSize: 12, Bold: true}},
					HSpacer{},
					Label{AssignTo: &verLabel, Text: "v"},
				},
			},
			TabWidget{
				Pages: []TabPage{
					{
						Title:  "基本设置",
						Layout: VBox{},
						Children: []Widget{
							Label{Text: "文件根目录"},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									LineEdit{AssignTo: &pathEdit},
									PushButton{
										Text: "浏览",
										OnClicked: func() {
											p, err := folderdialog.Pick(pathEdit.Text(), "选择文件根目录")
											if err == nil && strings.TrimSpace(p) != "" {
												_ = pathEdit.SetText(p)
											}
										},
									},
								},
							},
							Label{Text: "服务端口 (默认 8000)"},
							LineEdit{AssignTo: &portEdit},
							Label{Text: "管理端密码"},
							LineEdit{AssignTo: &pwdEdit},
							CheckBox{AssignTo: &autoOpenCB, Text: "启动后打开浏览器管理端", Checked: true},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{AssignTo: &btnStart, Text: "启动服务", OnClicked: func() {
										if _, err := doSave(readForm()); err != nil {
											walk.MsgBox(mw, "保存失败", err.Error(), walk.MsgBoxIconError)
											return
										}
										st, err := doStart(autoOpenCB.Checked())
										if err != nil {
											walk.MsgBox(mw, "无法启动", st.Message, walk.MsgBoxIconWarning)
										} else if strings.Contains(st.Message, "已自动改用") {
											walk.MsgBox(mw, "端口已调整", st.Message, walk.MsgBoxIconInformation)
										}
										applyState(st)
									}},
									PushButton{AssignTo: &btnStop, Text: "停止服务", OnClicked: func() { applyState(doStop()) }},
									PushButton{Text: "保存设置", OnClicked: func() {
										st, err := doSave(readForm())
										if err != nil {
											walk.MsgBox(mw, "保存失败", err.Error(), walk.MsgBoxIconError)
											return
										}
										applyState(st)
									}},
								},
							},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{Text: "保存并重启", OnClicked: func() {
										if _, err := doSave(readForm()); err != nil {
											walk.MsgBox(mw, "保存失败", err.Error(), walk.MsgBoxIconError)
											return
										}
										st, err := doRestart()
										if err != nil {
											walk.MsgBox(mw, "无法启动", st.Message, walk.MsgBoxIconWarning)
										} else if strings.Contains(st.Message, "已自动改用") {
											walk.MsgBox(mw, "端口已调整", st.Message, walk.MsgBoxIconInformation)
										}
										applyState(st)
									}},
									PushButton{AssignTo: &btnAdmin, Text: "打开管理端", OnClicked: func() {
										st := buildState("")
										if st.Running {
											_ = cmd.Open(st.AdminURL)
										}
									}},
									PushButton{AssignTo: &btnStudent, Text: "打开学生端", OnClicked: func() {
										st := buildState("")
										if st.Running {
											_ = cmd.Open(st.StudentURL)
										}
									}},
									PushButton{Text: "检查更新", OnClicked: func() {
										_ = tipLB.SetText("正在检查更新…")
										go runForceUpdateCheck(mw, tipLB, btnStart, btnStop, btnAdmin, btnStudent, true)
									}},
								},
							},
							Label{AssignTo: &tipLB, Text: " "},
							VSpacer{},
						},
					},
					{
						Title:  "运行日志",
						Layout: VBox{},
						Children: []Widget{
							Label{Text: "文件位置: " + resolveLogPath() + "（自动刷新）"},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{Text: "刷新", OnClicked: func() { _ = logEdit.SetText(applog.Tail(120 << 10)) }},
									PushButton{Text: "打开日志目录", OnClicked: func() {
										dir := filepath.Dir(resolveLogPath())
										_ = os.MkdirAll(dir, 0755)
										_ = cmd.Open(dir)
									}},
									PushButton{Text: "清空显示", OnClicked: func() { _ = logEdit.SetText("") }},
									HSpacer{},
								},
							},
							TextEdit{AssignTo: &logEdit, ReadOnly: true, VScroll: true},
						},
					},
				},
			},
			Label{AssignTo: &statusLabel, Text: "服务未启动", Font: Font{PointSize: 10, Bold: true}},
			Label{AssignTo: &linkLabel, Text: "启动服务后显示访问链接"},
		},
	}.Create()

	if err != nil {
		panelErr("面板创建失败: " + err.Error())
		return false
	}

	cfg := loadPanelConfig()
	_ = pathEdit.SetText(cfg.Path)
	_ = portEdit.SetText(cfg.Port)
	_ = pwdEdit.SetText(cfg.Password)
	autoOpenCB.SetChecked(cfg.AutoOpen)
	_ = verLabel.SetText("v" + passapp.AppVersion)
	applyState(buildState(""))

	// 窗口图标：优先用打进 exe 的资源；否则旁路 icon.ico
	if ico, err := walk.NewIconFromResourceId(2); err == nil {
		_ = mw.SetIcon(ico)
	} else if ico, err := walk.NewIconFromResourceId(3); err == nil {
		_ = mw.SetIcon(ico)
	} else if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "icon.ico")
		if ico, err := walk.NewIconFromFile(p); err == nil {
			_ = mw.SetIcon(ico)
		}
	}

	_ = logEdit.SetText(applog.Tail(120 << 10))

	// 先强制检查更新：有新版本则封锁面板并自动下载，不启动服务
	go func() {
		blocked := runForceUpdateCheck(mw, tipLB, btnStart, btnStop, btnAdmin, btnStudent, false)
		if blocked {
			return
		}
		st, _ := doStart(true)
		syncApply(st)
	}()

	// 运行日志每 2 秒自动刷新
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			if mw == nil {
				return
			}
			txt := applog.Tail(120 << 10)
			mw.Synchronize(func() {
				if logEdit != nil {
					_ = logEdit.SetText(txt)
				}
			})
		}
	}()

	mw.Run()
	_ = gateway.StopServer()
	return true
}

// runForceUpdateCheck 发现新版本时：停服务、锁按钮、弹窗、自动下载安装包。
// 返回 true 表示必须更新、不可继续用旧版。
func runForceUpdateCheck(
	mw *walk.MainWindow,
	tipLB *walk.Label,
	btnStart, btnStop, btnAdmin, btnStudent *walk.PushButton,
	manual bool,
) bool {
	u := passapp.CheckUpdateInfo()
	if u.CheckError != "" {
		applog.Info("更新检查失败: %s", u.CheckError)
		if mw != nil {
			mw.Synchronize(func() {
				if tipLB != nil {
					msg := "暂时无法检查更新（网络）"
					if manual {
						msg = "检查失败: " + u.CheckError
					}
					_ = tipLB.SetText(msg)
				}
				if manual {
					walk.MsgBox(mw, "检查更新", "暂时连不上更新服务器，请稍后重试或检查网络。", walk.MsgBoxIconWarning)
				}
			})
		}
		return false
	}
	if !u.NeedUpdate {
		if mw != nil && manual {
			mw.Synchronize(func() {
				if tipLB != nil {
					_ = tipLB.SetText("已是最新 v" + u.Current)
				}
				walk.MsgBox(mw, "检查更新", "当前已是最新版本 v"+u.Current, walk.MsgBoxIconInformation)
			})
		}
		return false
	}

	applog.Info("强制更新: 当前 v%s → 最新 v%s", u.Current, u.Latest)
	forceUpdateLocked = true
	_ = gateway.StopServer()

	if mw != nil {
		mw.Synchronize(func() {
			if tipLB != nil {
				_ = tipLB.SetText(fmt.Sprintf("必须更新到 v%s（当前 v%s）", u.Latest, u.Current))
			}
			if btnStart != nil {
				btnStart.SetEnabled(false)
			}
			if btnStop != nil {
				btnStop.SetEnabled(false)
			}
			if btnAdmin != nil {
				btnAdmin.SetEnabled(false)
			}
			if btnStudent != nil {
				btnStudent.SetEnabled(false)
			}
			walk.MsgBox(mw, "必须更新",
				fmt.Sprintf("发现新版本 v%s（当前 v%s）。\n\n旧版本不能继续使用，正在自动下载安装包…", u.Latest, u.Current),
				walk.MsgBoxIconWarning)
		})
	}

	dest, err := passapp.DownloadUpdateZip(u)
	if mw != nil {
		mw.Synchronize(func() {
			if err != nil {
				applog.Info("自动下载失败: %v", err)
				if tipLB != nil {
					_ = tipLB.SetText("自动下载失败，请点「检查更新」重试或打开下载页")
				}
				if u.DownloadURL != "" {
					_ = cmd.Open(u.DownloadURL)
				} else if u.ReleaseURL != "" {
					_ = cmd.Open(u.ReleaseURL)
				}
				walk.MsgBox(mw, "请手动下载",
					"自动下载失败，已尝试打开下载页。\n请下载 nextpass-win.zip，解压后覆盖本目录的 nextpass.exe，再重新打开。",
					walk.MsgBoxIconError)
				return
			}
			dir := filepath.Dir(dest)
			_ = cmd.Open(dir)
			if tipLB != nil {
				_ = tipLB.SetText("新版本已下载，请解压覆盖后重启")
			}
			walk.MsgBox(mw, "请完成更新",
				fmt.Sprintf("新版本已下载到：\n%s\n\n请解压，用新的 nextpass.exe 覆盖旧文件，然后重新打开本程序。\n覆盖完成前旧版本无法使用。", dest),
				walk.MsgBoxIconInformation)
		})
	}
	return true
}
