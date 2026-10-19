package handler

// T54 — обложка курса: POST/DELETE /teacher/courses/{id}/cover.
// Контракт — Task/00-api-contract.md, раздел «T54 — обложка курса (cover)»
// (заморожен). Файлы сохраняет storage.SaveLocal в uploads/covers/ — каталог
// уже раздаётся статикой маршрутом /uploads/* (см. NewRouter в middleware.go).
//
// Имя файла на диске привязано к курсу (courseId + расширение), поэтому
// повторная загрузка перезаписывает файл, а старые копии не накапливаются.
// Физическое удаление файла при DELETE контрактом не требуется — сбрасывается
// только поле cover_path.

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"edu-platform/backend/internal/repo"
	"edu-platform/backend/internal/storage"
)

// maxCoverBytes — максимум на сам файл обложки (замороженный контракт: 5 МБ).
const maxCoverBytes = 5 * 1024 * 1024

// maxCoverBodyBytes — локальный лимит тела multipart-запроса: файл до 5 МБ
// плюс запас ~1 МБ на служебные части формы. Ставится ТОЛЬКО в этом хендлере
// (http.MaxBytesReader): глобального «жёсткого» лимита тела на ветке /api/v1
// нет — 1 МБ применяет сам decodeJSON к JSON-декодеру, multipart он не
// задевает; здесь лимит — защита от заведомо невалидного мусора. Если когда-
// нибудь появится общий middleware лимита тела — для cover нужен пропуск.
const maxCoverBodyBytes = 6 * 1024 * 1024

// coverTypeExt — допустимые типы обложки по байтам тела
// (http.DetectContentType, контракт: image/jpeg|png|webp) и расширение имени
// файла на диске.
var coverTypeExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// coverBadFileErr — единый текст 400 по контракту: плохой тип или больше
// 5 МБ (а также прочие невалидные multipart-запросы).
const coverBadFileErr = "Обложка: допустимы JPEG, PNG или WebP не больше 5 МБ"

// coverArchivedErr — 409 для архивного курса (обложка, как карточка и
// программа, в архиве заморожена).
const coverArchivedErr = "Курс в архиве: обложка заморожена"

// coverStorage отдаёт хранилище файлов для обложек. nil → LocalStore в
// ./uploads: новая переменная окружения не заводится, стенд отдает этот
// каталог по /uploads/*, а тесты подставляют свою реализацию.
func (a *App) coverStorage() storage.ObjectStore {
	if a.Storage != nil {
		return a.Storage
	}
	return storage.LocalStore{}
}

// RegisterTeacherCoverRoutes вешает маршруты обложки на /teacher-роутер
// (requireAuth + requireRole(teacher, admin) уже навешены родителем,
// пути относительные — монтируется рядом с другими Register-методами).
func (a *App) RegisterTeacherCoverRoutes(r chi.Router) {
	r.Post("/courses/{id}/cover", a.TeacherUploadCover)
	r.Delete("/courses/{id}/cover", a.TeacherDeleteCover)
}

// saveCover пишет multipart-файл в хранилище и возвращает публичный путь
// «/uploads/covers/<courseId>.<ext>». Валидацию типа/размера делает
// TeacherUploadCover: здесь только сохранение по готовому имени.
func (a *App) saveCover(courseID string, ext string, data []byte) (string, error) {
	return a.coverStorage().SaveLocal("covers/"+courseID+ext, data)
}

// TeacherUploadCover — POST /courses/{id}/cover (multipart/form-data,
// обязательное поле file): тип файла определяется по байтам тела
// через http.DetectContentType, размер — по прочитанным байтам.
// Успех 201 {"coverPath":"/uploads/covers/<имя>"}, повторная загрузка
// затирает файл перезаписью и обновляет cover_path.
// Ошибки: 400 — нет файла/плохой тип/слишком большой; 401/403 — права;
// 404 — нет курса; 409 — курс в архиве.
func (a *App) TeacherUploadCover(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	// Статус ресурса важнее валидации тела (как в PATCH карточки,
	// teacher_edit.go): проверяем архив ДО разбора multipart — архивному
	// курсу обложка не меняется даже при пустом/битом теле.
	if c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, coverArchivedErr)
		return
	}
	// Локальный лимит тела (maxCoverBodyBytes) — только для этого маршрута.
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBodyBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, coverBadFileErr)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		// Обрыв чтения (в т.ч. сработавший MaxBytesReader) — тело не валидно.
		writeErr(w, http.StatusBadRequest, coverBadFileErr)
		return
	}
	if len(data) > maxCoverBytes {
		writeErr(w, http.StatusBadRequest, coverBadFileErr)
		return
	}
	ext, allowed := coverTypeExt[http.DetectContentType(data)]
	if !allowed {
		writeErr(w, http.StatusBadRequest, coverBadFileErr)
		return
	}
	path, err := a.saveCover(c.ID, ext, data)
	if err != nil {
		// Проблемы записи на диск — серверная, а не клиентская ошибка
		// (клиент свою часть — валидный multipart с картинкой — уже прошёл).
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить обложку")
		return
	}
	if !a.Store.SetCourseCover(c.ID, &path) {
		writeErr(w, http.StatusInternalServerError, "Не удалось сохранить обложку")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"coverPath": path})
}

// TeacherDeleteCover — DELETE /courses/{id}/cover: сброс cover_path в NULL,
// всегда 204 (обложки и не было — тот же ответ, идемпотентность по контракту).
// 409 — курс в архиве (заморожен); 404 — неизвестный курс; 403 — чужой.
func (a *App) TeacherDeleteCover(w http.ResponseWriter, r *http.Request) {
	_, c, ok := a.ownCourse(w, r)
	if !ok {
		return
	}
	if c.Status == repo.CourseArchived {
		writeErr(w, http.StatusConflict, coverArchivedErr)
		return
	}
	if !a.Store.SetCourseCover(c.ID, nil) {
		writeErr(w, http.StatusInternalServerError, "Не удалось удалить обложку")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
