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
		on := st.Running
		if on {
			_ = statusLabel.SetText("服务运行中")
			_ = linkLabel.SetText("学生: " + st.StudentURL + "\r\n管理: " + st.AdminURL)
		} else {
			_ = statusLabel.SetText("服务未启动")
			msg := st.Message
			if msg == "" {
				msg = "点击「启动服务」后，学生用浏览器访问"
			}
			_ = linkLabel.SetText(msg)
		}
		btnStart.SetEnabled(!on)
		btnStop.SetEnabled(on)
		btnAdmin.SetEnabled(on)
		btnStudent.SetEnabled(on)
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
											walk.MsgBox(mw, "启动失败", err.Error(), walk.MsgBoxIconError)
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
											walk.MsgBox(mw, "重启失败", err.Error(), walk.MsgBoxIconError)
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
										_ = tipLB.SetText("正在检查...")
										go func() {
											u := passapp.CheckUpdateInfo()
											mw.Synchronize(func() {
												if u.CheckError != "" {
													_ = tipLB.SetText("检查失败: " + u.CheckError)
													return
												}
												if u.NeedUpdate {
													_ = tipLB.SetText(fmt.Sprintf("有新版本 v%s (当前 v%s)", u.Latest, u.Current))
													if u.DownloadURL != "" && walk.MsgBox(mw, "发现新版本", "是否打开下载页?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
														_ = cmd.Open(u.DownloadURL)
													}
												} else {
													_ = tipLB.SetText("已是最新 v" + u.Current)
												}
											})
										}()
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

	go func() {
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
