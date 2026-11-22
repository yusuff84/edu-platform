package handler

// Медиафайлы для уроков (картинки-схемы и видео), загружаемые по токену MCP:
//
//	POST /api/v1/mcp/media  (Authorization: Bearer skx_…, multipart file=…)
//	→ {"url": "/uploads/media/<хеш>.<ext>", "contentType": "…", "bytes": N}
//
// и MCP-инструмент upload_media (тот же файл в base64). Имя файла — по хешу
// содержимого: повторная загрузка того же файла не плодит копий. Принимаются
// PNG, JPEG, WebP, GIF и MP4 до 5,5 МБ (лимит nginx — 6 МБ на запрос).

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
)

const maxMediaBytes = 5*1024*1024 + 512*1024

var mediaTypeExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"video/mp4":  ".mp4",
}

const mediaBadFileErr = "Медиа: допустимы PNG, JPEG, WebP, GIF или MP4 не больше 5,5 МБ"

// saveMedia проверяет тип по содержимому и сохраняет файл; возвращает публичный путь.
func (a *App) saveMedia(data []byte) (url, contentType string, status int, errText string) {
	if len(data) == 0 || len(data) > maxMediaBytes {
		return "", "", http.StatusBadRequest, mediaBadFileErr
	}
	contentType = http.DetectContentType(data)
	ext, ok := mediaTypeExt[contentType]
	if !ok {
		return "", "", http.StatusBadRequest, mediaBadFileErr
	}
	sum := sha256.Sum256(data)
	url, err := a.coverStorage().SaveLocal("media/"+hex.EncodeToString(sum[:12])+ext, data)
	if err != nil {
		return "", "", http.StatusInternalServerError, "Не удалось сохранить файл"
	}
	return url, contentType, http.StatusCreated, ""
}

// MCPUploadMedia — POST /api/v1/mcp/media.
func (a *App) MCPUploadMedia(w http.ResponseWriter, r *http.Request) {
	u, _, ok := a.mcpUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "MCP: нужен действующий токен администратора (Authorization: Bearer skx_…)")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaBytes+512*1024)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, mediaBadFileErr)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, mediaBadFileErr)
		return
	}
	url, ct, status, msg := a.saveMedia(data)
	if msg != "" {
		writeErr(w, status, msg)
		return
	}
	a.Store.AddAuditEntry(u.ID, u.Email, "Нейросеть (MCP): загружен файл "+url, clientIP(r))
	writeJSON(w, http.StatusCreated, map[string]any{"url": url, "contentType": ct, "bytes": len(data)})
}
