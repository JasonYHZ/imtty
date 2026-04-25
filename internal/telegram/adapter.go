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
		if len(fields) != 2 {
			return []string{"用法：/open <project>"}
		}

		if previous, ok := a.registry.Active(); ok && previous.Project != fields[1] {
			a.runtime.CloseSession(previous.Name)
		}

		view, err := a.registry.Open(fields[1])
		if err != nil {
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		if err := a.runtime.OpenSession(ctx, chatIDFromContext(ctx), view); err != nil {
			_, _ = a.registry.SetState(view.Project, session.StateLost)
			_ = a.registry.CloseActive()
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		view, err = a.registry.SetState(view.Project, session.StateRunning)
		if err != nil {
			return []string{fmt.Sprintf("打开项目失败：%v", err)}
		}

		return []string{fmt.Sprintf("已切换到会话 %s [%s]", view.Name, view.State)}
	case "/close":
		active, ok := a.registry.Active()
		if !ok {
			return []string{"当前没有可关闭的活跃会话"}
		}

		a.runtime.CloseSession(active.Name)

		if err := a.registry.CloseActive(); err != nil {
			return []string{fmt.Sprintf("关闭会话失败：%v", err)}
		}

		return []string{fmt.Sprintf("已关闭当前会话 %s", active.Name)}
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

		a.runtime.CloseSession(active.Name)
		if err := a.runtime.KillSession(ctx, active); err != nil {
			return []string{fmt.Sprintf("终止会话失败：%v", err)}
		}

		view, err := a.registry.Kill(active.Project)
		if err != nil {
			return []string{fmt.Sprintf("终止会话失败：%v", err)}
		}

		return []string{fmt.Sprintf("已彻底删除会话 %s", view.Name)}
	case "/status":
		return []string{a.renderStatus()}
	default:
		return []string{"未知命令；支持的命令有：/list、/projects、/project_add <name> <abs-path>、/project_remove <name>、/open <project>、/close、/kill、/status"}
	}
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

func (a *Adapter) renderStatus() string {
	lines := make([]string, 0, 4)

	if active, ok := a.registry.Active(); ok {
		lines = append(lines, fmt.Sprintf("当前会话: %s [%s]", active.Name, active.State))
	} else {
		lines = append(lines, "当前会话: 无")
	}

	sessions := a.registry.List()
	if len(sessions) == 0 {
		lines = append(lines, "会话列表: 无")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "会话列表：")
	for _, view := range sessions {
		lines = append(lines, fmt.Sprintf("- %s [%s]", view.Name, view.State))
	}

	return strings.Join(lines, "\n")
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
