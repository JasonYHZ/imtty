package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"imtty/internal/appserver"
	"imtty/internal/fileinput"
	"imtty/internal/session"
)

type Adapter struct {
	registry     *session.Registry
	runtime      SessionRuntime
	projectStore ProjectStore
	fileClient   FileClient
	mediaStore   MediaStore
	documenter   DocumentAnalyzer
}

type SessionRuntime interface {
	OpenSession(ctx context.Context, chatID int64, view session.View) error
	CloseSession(sessionName string)
	SubmitText(ctx context.Context, chatID int64, view session.View, text string) error
	SubmitImage(ctx context.Context, chatID int64, view session.View, imagePath string, caption string) error
	SubmitApproval(ctx context.Context, chatID int64, view session.View, text string) (bool, error)
	Status(ctx context.Context, view session.View) (session.Status, error)
	ListModels(ctx context.Context, view session.View) ([]appserver.ModelInfo, error)
	SetModel(ctx context.Context, view session.View, model string) (session.Status, string, error)
	SetReasoning(ctx context.Context, view session.View, reasoning string) (session.Status, error)
	SetPlanMode(ctx context.Context, view session.View, mode session.PlanMode) (session.Status, error)
	ClearThread(ctx context.Context, view session.View) (session.Status, error)
	IsLocallyAttached(ctx context.Context, sessionName string) (bool, error)
	KillSession(ctx context.Context, view session.View) error
}

type ProjectStore interface {
	AddProject(name string, root string) error
	RemoveProject(name string) error
}

type FileClient interface {
	GetFile(ctx context.Context, fileID string) (TelegramFile, error)
	DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error)
}

type MediaStore interface {
	SaveImage(sessionName string, fileID string, extension string, body io.Reader) (string, error)
	SaveDocument(sessionName string, fileID string, extension string, body io.Reader) (string, error)
}

type DocumentAnalyzer interface {
	BuildTurnText(ctx context.Context, path string, fileName string, mimeType string, caption string) (string, error)
}

func NewAdapter(registry *session.Registry, runtime SessionRuntime, projectStore ProjectStore, fileClient FileClient, mediaStore MediaStore, documenter DocumentAnalyzer) *Adapter {
	return &Adapter{
		registry:     registry,
		runtime:      runtime,
		projectStore: projectStore,
		fileClient:   fileClient,
		mediaStore:   mediaStore,
		documenter:   documenter,
	}
}

func (a *Adapter) HandleUpdate(ctx context.Context, update Update) []string {
	if update.Message == nil {
		return []string{"已忽略：缺少消息内容"}
	}

	ctx = withChatID(ctx, update.Message.Chat.ID)

	if attachment, handled, response := a.parseAttachment(update.Message); handled {
		return a.handleAttachmentMessage(ctx, attachment, response)
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return []string{"已忽略空消息"}
	}

	if strings.HasPrefix(text, "/") {
		return a.handleCommand(ctx, text)
	}

	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}

	attached, err := a.runtime.IsLocallyAttached(ctx, active.Name)
	if err != nil {
		return []string{fmt.Sprintf("检查会话占用失败：%v", err)}
	}
	if attached {
		return []string{"当前会话在桌面端本地占用中，请先 detach 后再继续远程操作"}
	}

	if handled, err := a.runtime.SubmitApproval(ctx, chatIDFromContext(ctx), active, text); handled {
		if err != nil {
			if err == appserver.ErrApprovalReplyRequired {
				return []string{err.Error()}
			}
			return []string{fmt.Sprintf("处理审批回复失败：%v", err)}
		}
		return nil
	} else if err != nil {
		return []string{fmt.Sprintf("处理审批回复失败：%v", err)}
	}

	if err := a.runtime.SubmitText(ctx, chatIDFromContext(ctx), active, text); err != nil {
		_, _ = a.registry.SetState(active.Project, session.StateLost)
		return []string{fmt.Sprintf("发送到会话 %s 失败：%v", active.Name, err)}
	}

	return nil
}

type attachmentKind string

const (
	attachmentImage    attachmentKind = "image"
	attachmentDocument attachmentKind = "document"
)

type attachmentReference struct {
	kind      attachmentKind
	fileID    string
	caption   string
	extension string
	fileName  string
	mimeType  string
}

func (a *Adapter) handleAttachmentMessage(ctx context.Context, attachment attachmentReference, earlyResponse []string) []string {
	if len(earlyResponse) > 0 {
		return earlyResponse
	}
	if a.fileClient == nil || a.mediaStore == nil {
		return []string{"文件消息当前不可用，请稍后再试"}
	}

	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}

	attached, err := a.runtime.IsLocallyAttached(ctx, active.Name)
	if err != nil {
		return []string{fmt.Sprintf("检查会话占用失败：%v", err)}
	}
	if attached {
		return []string{"当前会话在桌面端本地占用中，请先 detach 后再继续远程操作"}
	}

	file, err := a.fileClient.GetFile(ctx, attachment.fileID)
	if err != nil {
		return []string{fmt.Sprintf("获取 Telegram 文件失败：%v", err)}
	}

	body, err := a.fileClient.DownloadFile(ctx, file.FilePath)
	if err != nil {
		return []string{fmt.Sprintf("下载 Telegram 文件失败：%v", err)}
	}
	defer body.Close()

	switch attachment.kind {
	case attachmentImage:
		imagePath, err := a.mediaStore.SaveImage(active.Name, attachment.fileID, chooseImageExtension(attachment.extension, file.FilePath), body)
		if err != nil {
			return []string{fmt.Sprintf("保存临时图片失败：%v", err)}
		}

		if err := a.runtime.SubmitImage(ctx, chatIDFromContext(ctx), active, imagePath, attachment.caption); err != nil {
			_, _ = a.registry.SetState(active.Project, session.StateLost)
			return []string{fmt.Sprintf("发送图片到会话 %s 失败：%v", active.Name, err)}
		}
	case attachmentDocument:
		if a.documenter == nil {
			return []string{"文件消息当前不可用，请稍后再试"}
		}
		documentPath, err := a.mediaStore.SaveDocument(active.Name, attachment.fileID, chooseDocumentExtension(attachment.extension, file.FilePath), body)
		if err != nil {
			return []string{fmt.Sprintf("保存临时文件失败：%v", err)}
		}
		turnText, err := a.documenter.BuildTurnText(ctx, documentPath, attachment.fileName, attachment.mimeType, attachment.caption)
		if err != nil {
			switch {
			case errors.Is(err, fileinput.ErrUnsupportedFileType):
				return []string{"当前只支持图片、文本文件和 PDF"}
			case errors.Is(err, fileinput.ErrFileTooLarge):
				return []string{"文件过大，当前仅支持 1MB 以内文件"}
			case errors.Is(err, fileinput.ErrPDFTextExtractFailure):
				return []string{"PDF 文本提取失败，请稍后重试"}
			default:
				return []string{fmt.Sprintf("处理文件内容失败：%v", err)}
			}
		}
		if err := a.runtime.SubmitText(ctx, chatIDFromContext(ctx), active, turnText); err != nil {
			_, _ = a.registry.SetState(active.Project, session.StateLost)
			return []string{fmt.Sprintf("发送文件到会话 %s 失败：%v", active.Name, err)}
		}
	}
	return nil
}

func (a *Adapter) parseAttachment(message *Message) (attachmentReference, bool, []string) {
	if message == nil {
		return attachmentReference{}, false, nil
	}
	if len(message.Photo) > 0 {
		photo := largestPhoto(message.Photo)
		return attachmentReference{
			kind:      attachmentImage,
			fileID:    photo.FileID,
			caption:   strings.TrimSpace(message.Caption),
			extension: ".jpg",
		}, true, nil
	}
	if message.Document == nil {
		return attachmentReference{}, false, nil
	}

	mime := strings.ToLower(strings.TrimSpace(message.Document.MimeType))
	if isSupportedDocumentImage(mime) {
		return attachmentReference{
			kind:      attachmentImage,
			fileID:    message.Document.FileID,
			caption:   strings.TrimSpace(message.Caption),
			extension: imageExtensionFromDocument(*message.Document),
			fileName:  message.Document.FileName,
			mimeType:  message.Document.MimeType,
		}, true, nil
	}

	return attachmentReference{
		kind:      attachmentDocument,
		fileID:    message.Document.FileID,
		caption:   strings.TrimSpace(message.Caption),
		extension: documentExtensionFromDocument(*message.Document),
		fileName:  message.Document.FileName,
		mimeType:  message.Document.MimeType,
	}, true, nil
}

func largestPhoto(photos []PhotoSize) PhotoSize {
	best := photos[0]
	for _, photo := range photos[1:] {
		if photo.FileSize > best.FileSize {
			best = photo
		}
	}
	return best
}

func isSupportedDocumentImage(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func imageExtensionFromDocument(document Document) string {
	if extension := strings.ToLower(filepath.Ext(document.FileName)); extension != "" {
		return extension
	}
	switch strings.ToLower(strings.TrimSpace(document.MimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func chooseImageExtension(preferred string, filePath string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	if extension := strings.ToLower(filepath.Ext(filePath)); extension != "" {
		return extension
	}
	return ".bin"
}

func documentExtensionFromDocument(document Document) string {
	if extension := strings.ToLower(filepath.Ext(document.FileName)); extension != "" {
		return extension
	}
	return ".bin"
}

func chooseDocumentExtension(preferred string, filePath string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	if extension := strings.ToLower(filepath.Ext(filePath)); extension != "" {
		return extension
	}
	return ".bin"
}

func (a *Adapter) handleCommand(ctx context.Context, text string) []string {
	fields := strings.Fields(text)
	command := fields[0]

	switch command {
	case "/list":
		return []string{a.renderList()}
	case "/projects":
		return []string{a.renderProjects()}
	case "/project_add":
		return a.handleProjectAdd(text)
	case "/project_remove":
		return a.handleProjectRemove(ctx, fields)
	case "/open":
		if len(fields) != 2 && len(fields) != 3 {
			return []string{"用法：/open <project> [thread-id]"}
		}

		project := fields[1]
		expectedThreadID := ""
		if len(fields) == 3 {
			expectedThreadID = strings.TrimSpace(fields[2])
		}

		if current, ok := a.registry.Active(); ok && current.Project == project {
			status, err := a.runtime.Status(ctx, current)
			if err == nil {
				if expectedThreadID != "" && status.ThreadID != expectedThreadID {
					return []string{strings.Join([]string{
						"打开项目失败：thread id 不匹配",
						fmt.Sprintf("当前 thread: %s", status.ThreadID),
						fmt.Sprintf("下一步: 使用 /open %s %s", project, status.ThreadID),
					}, "\n")}
				}
				return []string{fmt.Sprintf("已切换到会话 %s [%s]", current.Name, current.State)}
			}
		}

		preview, err := a.registry.Resolve(project)
		if err != nil {
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		if err := a.runtime.OpenSession(ctx, chatIDFromContext(ctx), preview); err != nil {
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		status, err := a.runtime.Status(ctx, preview)
		if err != nil {
			a.runtime.CloseSession(preview.Name)
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}
		if expectedThreadID != "" && status.ThreadID != expectedThreadID {
			a.runtime.CloseSession(preview.Name)
			return []string{strings.Join([]string{
				"打开项目失败：thread id 不匹配",
				fmt.Sprintf("当前 thread: %s", status.ThreadID),
				fmt.Sprintf("下一步: 使用 /open %s %s", project, status.ThreadID),
			}, "\n")}
		}

		if previous, ok := a.registry.Active(); ok && previous.Project != project {
			a.runtime.CloseSession(previous.Name)
		}

		view, err := a.registry.Open(project)
		if err != nil {
			a.runtime.CloseSession(preview.Name)
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		view, err = a.registry.SetState(view.Project, session.StateRunning)
		if err != nil {
			a.runtime.CloseSession(preview.Name)
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		return []string{fmt.Sprintf("已切换到会话 %s [%s]", view.Name, view.State)}
	case "/close":
		active, ok := a.registry.Active()
		if !ok {
			return []string{"当前没有可关闭的活跃会话"}
		}

		status, statusErr := a.runtime.Status(ctx, active)
		a.runtime.CloseSession(active.Name)

		if err := a.registry.CloseActive(); err != nil {
			return []string{fmt.Sprintf("关闭会话失败：%v", err)}
		}

		lines := []string{fmt.Sprintf("已关闭当前会话 %s", active.Name)}
		if statusErr == nil && strings.TrimSpace(status.ThreadID) != "" {
			lines = append([]string{fmt.Sprintf("当前 thread: %s", status.ThreadID)}, lines...)
		}
		return []string{strings.Join(lines, "\n")}
	case "/kill":
		active, ok := a.registry.Active()
		if !ok {
			return []string{"当前没有可终止的活跃会话"}
		}

		attached, err := a.runtime.IsLocallyAttached(ctx, active.Name)
		if err != nil {
			return []string{fmt.Sprintf("检查会话占用失败：%v", err)}
		}
		if attached {
			return []string{"当前会话在桌面端本地占用中，请先 detach 后再继续远程操作"}
		}

		status, statusErr := a.runtime.Status(ctx, active)
		a.runtime.CloseSession(active.Name)
		if err := a.runtime.KillSession(ctx, active); err != nil {
			return []string{fmt.Sprintf("终止会话失败：%v", err)}
		}

		view, err := a.registry.Kill(active.Project)
		if err != nil {
			return []string{fmt.Sprintf("终止会话失败：%v", err)}
		}

		lines := []string{fmt.Sprintf("已彻底删除会话 %s", view.Name)}
		if statusErr == nil && strings.TrimSpace(status.ThreadID) != "" {
			lines = append([]string{fmt.Sprintf("当前 thread: %s", status.ThreadID)}, lines...)
		}
		return []string{strings.Join(lines, "\n")}
	case "/clear":
		return a.handleClearCommand(ctx)
	case "/status":
		return []string{a.renderStatus(ctx)}
	case "/model":
		return a.handleModelCommand(ctx, fields)
	case "/reasoning":
		return a.handleReasoningCommand(ctx, fields)
	case "/plan_mode":
		return a.handlePlanModeCommand(ctx, fields)
	default:
		return []string{"未知命令；支持的命令有：/list、/projects、/project_add <name> <abs-path>、/project_remove <name>、/open <project> [thread-id]、/close、/kill、/clear、/status、/model [model]、/reasoning [effort]、/plan_mode [default|plan]"}
	}
}

func (a *Adapter) handleClearCommand(ctx context.Context) []string {
	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}
	if response := a.ensureRemoteControlAllowed(ctx, active); response != nil {
		return response
	}

	status, err := a.runtime.ClearThread(ctx, active)
	if err != nil {
		return []string{fmt.Sprintf("清空当前对话失败：%v", err)}
	}

	lines := []string{
		"已清空当前对话上下文",
		fmt.Sprintf("新 thread: %s", status.ThreadID),
		"下一步: 直接发送新消息即可",
	}
	return []string{strings.Join(lines, "\n")}
}

func (a *Adapter) renderList() string {
	sessions := a.registry.List()
	if len(sessions) == 0 {
		return "会话列表: 无"
	}

	lines := []string{"会话列表："}
	for _, view := range sessions {
		lines = append(lines, fmt.Sprintf("- %s [%s]", view.Name, view.State))
	}

	return strings.Join(lines, "\n")
}

func (a *Adapter) handleProjectAdd(text string) []string {
	name, root, ok := parseProjectAdd(text)
	if !ok {
		return []string{"用法：/project_add <name> <abs-path>"}
	}

	if !filepath.IsAbs(root) {
		return []string{"添加项目失败：路径必须是绝对路径"}
	}

	if err := a.registry.AddAllowedProject(name, root); err != nil {
		return []string{fmt.Sprintf("添加项目失败：%v", err)}
	}

	if err := a.projectStore.AddProject(name, root); err != nil {
		_, _, _ = a.registry.RemoveAllowedProject(name)
		return []string{fmt.Sprintf("添加项目失败：%v", err)}
	}

	return []string{fmt.Sprintf("已添加项目 %s => %s", name, root)}
}

func (a *Adapter) handleProjectRemove(ctx context.Context, fields []string) []string {
	if len(fields) != 2 {
		return []string{"用法：/project_remove <name>"}
	}

	view, hadSession, err := a.registry.RemoveAllowedProject(fields[1])
	if err != nil {
		return []string{fmt.Sprintf("移除项目失败：%v", err)}
	}

	if err := a.projectStore.RemoveProject(fields[1]); err != nil {
		return []string{fmt.Sprintf("移除项目失败：%v", err)}
	}

	if hadSession {
		attached, err := a.runtime.IsLocallyAttached(ctx, view.Name)
		if err != nil {
			return []string{fmt.Sprintf("移除项目失败：%v", err)}
		}
		if attached {
			return []string{"当前会话在桌面端本地占用中，请先 detach 后再移除项目"}
		}
		a.runtime.CloseSession(view.Name)
		if err := a.runtime.KillSession(ctx, view); err != nil {
			return []string{fmt.Sprintf("移除项目失败：%v", err)}
		}
	}

	return []string{fmt.Sprintf("已移除项目 %s", fields[1])}
}

func (a *Adapter) renderProjects() string {
	allowed := a.registry.AllowedProjects()
	projectNames := make([]string, 0, len(allowed))
	for name := range allowed {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)

	lines := []string{"项目列表："}
	for _, name := range projectNames {
		lines = append(lines, fmt.Sprintf("- %s => %s", name, allowed[name]))
	}

	return strings.Join(lines, "\n")
}

func (a *Adapter) handleModelCommand(ctx context.Context, fields []string) []string {
	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}

	if len(fields) == 1 {
		status, err := a.runtime.Status(ctx, active)
		if err != nil {
			return []string{fmt.Sprintf("读取会话状态失败：%v", err)}
		}
		models, err := a.runtime.ListModels(ctx, active)
		if err != nil {
			return []string{fmt.Sprintf("读取模型列表失败：%v", err)}
		}
		return []string{a.renderModelStatus(status, models)}
	}
	if len(fields) != 2 {
		return []string{"用法：/model [model]"}
	}
	if response := a.ensureRemoteControlAllowed(ctx, active); response != nil {
		return response
	}
	status, note, err := a.runtime.SetModel(ctx, active, fields[1])
	if err != nil {
		return []string{fmt.Sprintf("设置模型失败：%v", err)}
	}
	lines := []string{
		fmt.Sprintf("已设置待生效模型：%s", effectiveOrPending(status.Pending.Model, status.Effective.Model)),
		"将在下一条消息时生效",
	}
	if strings.TrimSpace(note) != "" {
		lines = append(lines, note)
	}
	return []string{strings.Join(lines, "\n")}
}

func (a *Adapter) handleReasoningCommand(ctx context.Context, fields []string) []string {
	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}

	if len(fields) == 1 {
		status, err := a.runtime.Status(ctx, active)
		if err != nil {
			return []string{fmt.Sprintf("读取会话状态失败：%v", err)}
		}
		models, err := a.runtime.ListModels(ctx, active)
		if err != nil {
			return []string{fmt.Sprintf("读取模型列表失败：%v", err)}
		}
		return []string{a.renderReasoningStatus(status, models)}
	}
	if len(fields) != 2 {
		return []string{"用法：/reasoning [effort]"}
	}
	if response := a.ensureRemoteControlAllowed(ctx, active); response != nil {
		return response
	}
	status, err := a.runtime.SetReasoning(ctx, active, fields[1])
	if err != nil {
		return []string{fmt.Sprintf("设置 reasoning 失败：%v", err)}
	}
	return []string{strings.Join([]string{
		fmt.Sprintf("已设置待生效 reasoning：%s", effectiveOrPending(status.Pending.Reasoning, status.Effective.Reasoning)),
		"将在下一条消息时生效",
	}, "\n")}
}

func (a *Adapter) handlePlanModeCommand(ctx context.Context, fields []string) []string {
	active, ok := a.registry.Active()
	if !ok {
		return []string{"当前没有活跃会话，请先执行 /open <project>"}
	}

	if len(fields) == 1 {
		status, err := a.runtime.Status(ctx, active)
		if err != nil {
			return []string{fmt.Sprintf("读取会话状态失败：%v", err)}
		}
		return []string{a.renderPlanModeStatus(status)}
	}
	if len(fields) != 2 {
		return []string{"用法：/plan_mode [default|plan]"}
	}
	if response := a.ensureRemoteControlAllowed(ctx, active); response != nil {
		return response
	}

	mode := session.PlanMode(strings.TrimSpace(fields[1]))
	if mode != session.PlanModeDefault && mode != session.PlanModePlan {
		return []string{"用法：/plan_mode [default|plan]"}
	}
	status, err := a.runtime.SetPlanMode(ctx, active, mode)
	if err != nil {
		return []string{fmt.Sprintf("设置 plan mode 失败：%v", err)}
	}
	return []string{strings.Join([]string{
		fmt.Sprintf("已设置待生效计划模式：%s", effectiveOrPending(string(status.Pending.PlanMode), string(status.Effective.PlanMode))),
		fmt.Sprintf("待生效 reasoning：%s", effectiveOrPending(status.Pending.Reasoning, status.Effective.Reasoning)),
		"将在下一条消息时生效",
	}, "\n")}
}

func (a *Adapter) renderStatus(ctx context.Context) string {
	active, ok := a.registry.Active()
	if !ok {
		return "当前会话: 无\n下一步: 先执行 /open <project>"
	}

	status, err := a.runtime.Status(ctx, active)
	if err != nil {
		return fmt.Sprintf("读取会话状态失败：%v", err)
	}

	lines := []string{
		fmt.Sprintf("当前会话: %s [%s]", status.View.Name, status.View.State),
		fmt.Sprintf("模型: %s", status.Effective.Model),
		fmt.Sprintf("Reasoning: %s", status.Effective.Reasoning),
		fmt.Sprintf("计划模式: %s", status.Effective.PlanMode),
	}
	if status.Pending.Model != "" {
		lines = append(lines, fmt.Sprintf("待生效模型: %s", status.Pending.Model))
	}
	if status.Pending.Reasoning != "" {
		lines = append(lines, fmt.Sprintf("待生效 Reasoning: %s", status.Pending.Reasoning))
	}
	if status.Pending.PlanMode != "" {
		lines = append(lines, fmt.Sprintf("待生效计划模式: %s", status.Pending.PlanMode))
	}
	if strings.TrimSpace(status.Cwd) != "" {
		lines = append(lines, fmt.Sprintf("工作目录: %s", status.Cwd))
	}
	if strings.TrimSpace(status.Branch) != "" {
		lines = append(lines, fmt.Sprintf("分支: %s", status.Branch))
	}
	if strings.TrimSpace(status.CodexVersion) != "" {
		lines = append(lines, fmt.Sprintf("Codex CLI: %s", status.CodexVersion))
	}
	if strings.TrimSpace(status.ThreadID) != "" {
		lines = append(lines, fmt.Sprintf("Thread ID: %s", status.ThreadID))
	}
	if status.LocalWritableAttach {
		lines = append(lines, "桌面占用: 有可写 attach")
	} else {
		lines = append(lines, "桌面占用: 无")
	}
	if status.HasTokenUsage && status.TokenUsage.ContextWindow > 0 {
		lines = append(lines,
			fmt.Sprintf("Context 剩余: %d%%", contextPercentLeft(status.TokenUsage)),
			fmt.Sprintf("窗口大小: %s", humanTokenCount(status.TokenUsage.ContextWindow)),
			fmt.Sprintf("已用 Tokens: %s", humanTokenCount(status.TokenUsage.TotalTokens)),
		)
	}
	return strings.Join(lines, "\n")
}

func (a *Adapter) renderModelStatus(status session.Status, models []appserver.ModelInfo) string {
	lines := []string{
		fmt.Sprintf("当前模型: %s", status.Effective.Model),
	}
	if status.Pending.Model != "" {
		lines = append(lines, fmt.Sprintf("待生效模型: %s", status.Pending.Model))
	}
	lines = append(lines, "可用模型：")
	for _, model := range models {
		lines = append(lines, fmt.Sprintf("- %s [默认 reasoning: %s; 支持: %s]", model.Model, model.DefaultReasoning, strings.Join(model.Supported, ", ")))
	}
	return strings.Join(lines, "\n")
}

func (a *Adapter) renderReasoningStatus(status session.Status, models []appserver.ModelInfo) string {
	targetModel := effectiveOrPending(status.Pending.Model, status.Effective.Model)
	lines := []string{
		fmt.Sprintf("当前 reasoning: %s", status.Effective.Reasoning),
		fmt.Sprintf("目标模型: %s", targetModel),
	}
	if status.Pending.Reasoning != "" {
		lines = append(lines, fmt.Sprintf("待生效 reasoning: %s", status.Pending.Reasoning))
	}
	if model, ok := findModelInfo(models, targetModel); ok {
		lines = append(lines, fmt.Sprintf("支持的 reasoning: %s", strings.Join(model.Supported, ", ")))
	}
	return strings.Join(lines, "\n")
}

func (a *Adapter) renderPlanModeStatus(status session.Status) string {
	lines := []string{
		fmt.Sprintf("当前计划模式: %s", status.Effective.PlanMode),
	}
	if status.Pending.PlanMode != "" {
		lines = append(lines, fmt.Sprintf("待生效计划模式: %s", status.Pending.PlanMode))
	}
	return strings.Join(lines, "\n")
}

func (a *Adapter) ensureRemoteControlAllowed(ctx context.Context, active session.View) []string {
	attached, err := a.runtime.IsLocallyAttached(ctx, active.Name)
	if err != nil {
		return []string{fmt.Sprintf("检查会话占用失败：%v", err)}
	}
	if attached {
		return []string{"当前会话在桌面端本地占用中，请先 detach 后再继续远程操作"}
	}
	return nil
}

func findModelInfo(models []appserver.ModelInfo, target string) (appserver.ModelInfo, bool) {
	for _, model := range models {
		if model.Model == target || model.ID == target {
			return model, true
		}
	}
	return appserver.ModelInfo{}, false
}

func effectiveOrPending(pending string, effective string) string {
	if strings.TrimSpace(pending) != "" {
		return strings.TrimSpace(pending)
	}
	return strings.TrimSpace(effective)
}

func contextPercentLeft(tokens session.TokenUsage) int64 {
	if tokens.ContextWindow <= 0 {
		return 0
	}
	remaining := tokens.ContextWindow - tokens.TotalTokens
	if remaining < 0 {
		remaining = 0
	}
	return (remaining * 100) / tokens.ContextWindow
}

func humanTokenCount(value int64) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%dM", value/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%dK", value/1_000)
	default:
		return fmt.Sprintf("%d", value)
	}
}

type chatIDContextKey struct{}

func withChatID(ctx context.Context, chatID int64) context.Context {
	return context.WithValue(ctx, chatIDContextKey{}, chatID)
}

func chatIDFromContext(ctx context.Context) int64 {
	value, _ := ctx.Value(chatIDContextKey{}).(int64)
	return value
}

func parseProjectAdd(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", false
	}

	firstSpace := strings.IndexByte(trimmed, ' ')
	if firstSpace < 0 {
		return "", "", false
	}

	args := strings.TrimSpace(trimmed[firstSpace+1:])
	if args == "" {
		return "", "", false
	}

	secondSpace := strings.IndexByte(args, ' ')
	if secondSpace < 0 {
		return "", "", false
	}

	name := strings.TrimSpace(args[:secondSpace])
	root := strings.TrimSpace(args[secondSpace+1:])
	if name == "" || root == "" {
		return "", "", false
	}

	return name, root, true
}
