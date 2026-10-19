package contract_test

// Контрактные тесты обложки курса (волна P7, T54):
// POST/DELETE /teacher/courses/{id}/cover и coverPath (string|null) в видах.
// Источник правды — Task/00-api-contract.md, раздел «T54 — обложка курса
// (cover)» (заморожен). Хранилище файлов подменяется двойником: контрактные
// тесты не должны писать в uploads/ рабочего каталога.

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"edu-platform/backend/internal/handler"
	"edu-platform/backend/internal/repo"
)

// fakeCoverStore — двойник storage.ObjectStore: файлы копятся в карте,
// публичный путь строится так же, как у storage.LocalStore («/uploads/» + path).
type fakeCoverStore struct{ files map[string][]byte }

func (f *fakeCoverStore) SaveLocal(path string, data []byte) (string, error) {
	if f.files == nil {
		f.files = make(map[string][]byte)
	}
	f.files[path] = append([]byte(nil), data...)
	return "/uploads/" + path, nil
}

// Сигнатуры картинок: http.DetectContentType распознаёт тип по байтам начала
// файла, поэтому до честных изображений достаточно магических заголовков.
var (
	coverJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xDB}, bytes.Repeat([]byte{0x5A}, 64)...)
	coverPNG  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 32)...)
	// WebP: DetectContentType (net/http/sniff.go) распознаёт сигнатуру
	// «RIFF????WEBPVP»: только «RIFF…WEBP» (без «VP») он считает мусором.
	coverWebP = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0x00}, 16)...)
)

// postCover отправляет multipart-запрос POST /teacher/courses/{id}/cover.
// data == nil — форма БЕЗ поля file (проверка «файл не передан» → 400).
func postCover(t *testing.T, router http.Handler, courseID string, filename string, data []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if data != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("запись части multipart: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/cover", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// seedTeacherCourse: учитель и его курс-черновик (обложку можно грузить в
// любом статусе, кроме archived — модерация тут не нужна).
func seedTeacherCourse(t *testing.T, app *handler.App, router http.Handler, tag string) (teacherCookies []*http.Cookie, courseID string) {
	t.Helper()
	teacherCookies, _ = registerUser(t, router, "cover-"+tag+"@example.com", "password123", "")
	if !app.Store.SetUserRole(userIDByEmail(t, app.Store, "cover-"+tag+"@example.com"), repo.RoleTeacher) {
		t.Fatal("не удалось выдать роль teacher")
	}
	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses", map[string]any{
		"title": "Обложка " + tag, "description": "Описание курса с обложкой", "priceCents": 1000,
	}, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var course map[string]any
	decodeBody(t, rec, &course)
	courseID, _ = course["id"].(string)
	if courseID == "" {
		t.Fatalf("create course вернул пустой id: %s", rec.Body.String())
	}
	return teacherCookies, courseID
}

// Хэппи-путь: загрузка (тип/размер в норме, имя = courseId + расширение),
// перезапись повторной загрузкой и удаление с идемпотентным 204.
func TestCoverUploadReplaceDelete(t *testing.T) {
	app, router := newTestApp()
	covers := &fakeCoverStore{}
	app.Storage = covers
	teacherCookies, courseID := seedTeacherCourse(t, app, router, "happy")

	// Сначала WebP: путь привязан к курсу, файл ушёл в хранилище.
	rec := postCover(t, router, courseID, "cover.webp", coverWebP, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload webp: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeBody(t, rec, &created)
	want := "/uploads/covers/" + courseID + ".webp"
	if created["coverPath"] != want {
		t.Fatalf("coverPath=%v, ждали %q", created["coverPath"], want)
	}
	if len(covers.files) != 1 || !bytes.Equal(covers.files["covers/"+courseID+".webp"], coverWebP) {
		t.Fatalf("файл не сохранён как должен: %v", covers.files)
	}

	// Перезапись: повторная загрузка JPEG затирает файл курса.
	rec = postCover(t, router, courseID, "cover.jpg", coverJPEG, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload replace: status=%d body=%s", rec.Code, rec.Body.String())
	}
	replace := map[string]any{}
	decodeBody(t, rec, &replace)
	want = "/uploads/covers/" + courseID + ".jpg"
	if replace["coverPath"] != want {
		t.Fatalf("после перезаписи coverPath=%v, ждали %q", replace["coverPath"], want)
	}
	if len(covers.files) != 2 || !bytes.Equal(covers.files["covers/"+courseID+".jpg"], coverJPEG) {
		t.Fatalf("перезапись не сработала: %v", covers.files)
	}

	// PATCH карточки НЕ затирает обложку (правится только title).
	rec = doRequest(t, router, http.MethodPatch, "/api/v1/teacher/courses/"+courseID, map[string]any{
		"title": "Новое название курса",
	}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", rec.Code, rec.Body.String())
	}
	patched := map[string]any{}
	decodeBody(t, rec, &patched)
	if patched["coverPath"] != want {
		t.Fatalf("PATCH затёр coverPath: %v", patched["coverPath"])
	}

	// Удаление: 204, затем снова 204 (обложки нет — тот же ответ).
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/cover", nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete cover: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/cover", nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("повторное delete cover: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// После удаления coverPath — null (ключ есть, значение nil).
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID, nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher get: status=%d", rec.Code)
	}
	detail := map[string]any{}
	decodeBody(t, rec, &detail)
	course := detail["course"].(map[string]any)
	if v, ok := course["coverPath"]; !ok || v != nil {
		t.Fatalf("после удаления coverPath=%v (ok=%v), ждали null", v, ok)
	}
}

// coverPath аддитивно появляется во всех видах каталога и списков.
func TestCoverPathInViewResponses(t *testing.T) {
	app, router := newTestApp()
	covers := &fakeCoverStore{}
	app.Storage = covers
	// Опубликованный курс: каталог отдаёт его публично.
	_, courseID, slug := seedPublishedCourse(t, app, router, 50000)
	teacherCookies := loginUser(t, router, "teacher@example.com", "password123")
	adminCookies := loginUser(t, router, "admin@example.com", "password123")

	rec := postCover(t, router, courseID, "card.jpg", coverPNG, teacherCookies)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: status=%d body=%s", rec.Code, rec.Body.String())
	}
	want := "/uploads/covers/" + courseID + ".png"

	// Каталог: каждый элемент может нести coverPath; у нашего курса он есть.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog: status=%d", rec.Code)
	}
	var catalog struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &catalog)
	found := false
	for _, item := range catalog.Items {
		if item["id"] == courseID {
			found = true
			if item["coverPath"] != want {
				t.Fatalf("каталог coverPath=%v, ждали %q", item["coverPath"], want)
			}
		}
	}
	if !found {
		t.Fatal("опубликованный курс не попал в каталог")
	}

	// Публичная страница курса: course.coverPath.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses/"+slug, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("course page: status=%d", rec.Code)
	}
	page := map[string]any{}
	decodeBody(t, rec, &page)
	course := page["course"].(map[string]any)
	if course["coverPath"] != want {
		t.Fatalf("курс coverPath=%v, ждали %q", course["coverPath"], want)
	}

	// Список учителя.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses", nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher list: status=%d", rec.Code)
	}
	var teacherList struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &teacherList)
	if len(teacherList.Items) == 0 || teacherList.Items[0]["coverPath"] != want {
		t.Fatalf("teacher list coverPath нарушен: %s", rec.Body.String())
	}

	// Карточка учителя.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID, nil, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher get: status=%d", rec.Code)
	}
	teacherDetail := map[string]any{}
	decodeBody(t, rec, &teacherDetail)
	if teacherDetail["course"].(map[string]any)["coverPath"] != want {
		t.Fatalf("teacher get coverPath нарушен: %s", rec.Body.String())
	}

	// Список админа (T48): items[].coverPath.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/admin/courses", nil, adminCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var adminList struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &adminList)
	found = false
	for _, item := range adminList.Items {
		if item["id"] == courseID {
			found = true
			if item["coverPath"] != want {
				t.Fatalf("админский список coverPath=%v, ждали %q", item["coverPath"], want)
			}
		}
	}
	if !found {
		t.Fatal("курс не попал в админский список")
	}

	// Удаление обложки у опубликованного курса: каталог снова отдаёт null.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/cover", nil, teacherCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete cover: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodGet, "/api/v1/courses", nil, nil)
	var afterDelete struct {
		Items []map[string]any `json:"items"`
	}
	decodeBody(t, rec, &afterDelete)
	for _, item := range afterDelete.Items {
		if item["id"] == courseID {
			if v, ok := item["coverPath"]; !ok || v != nil {
				t.Fatalf("каталог после удаления coverPath=%v (ok=%v), ждали null", v, ok)
			}
		}
	}
}

// Валидация: нет файла → 400, мусорный тип → 400, больше 5 МБ → 400,
// неизвестный курс → 404.
func TestCoverUploadValidation(t *testing.T) {
	app, router := newTestApp()
	app.Storage = &fakeCoverStore{}
	teacherCookies, courseID := seedTeacherCourse(t, app, router, "validation")
	const badFileMsg = "Обложка: допустимы JPEG, PNG или WebP не больше 5 МБ"

	// М multipart без поля file.
	rec := postCover(t, router, courseID, "", nil, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("нет файла: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Мусорный тип по байтам (текст вместо картинки).
	rec = postCover(t, router, courseID, "note.txt", []byte("это просто текст, а не изображение"), teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("мусорный тип: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if errBody["error"] != badFileMsg {
		t.Fatalf("текст 400 мусорного типа=%v, ждали %q", errBody["error"], badFileMsg)
	}

	// 5,5 МБ: тип допустимый, размер — нет (лимит тела 6 МБ multipart не режет).
	big := make([]byte, 5*1024*1024+512*1024)
	copy(big, coverJPEG)
	rec = postCover(t, router, courseID, "big.jpg", big, teacherCookies)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf(">5 МБ: status=%d body=%s", rec.Code, rec.Body.String())
	}
	errBody = map[string]any{}
	decodeBody(t, rec, &errBody)
	if errBody["error"] != badFileMsg {
		t.Fatalf("текст 400 слишком большой=%v, ждали %q", errBody["error"], badFileMsg)
	}

	// Неизвестный курс — 404.
	rec = postCover(t, router, "00000000-0000-0000-0000-000000000000", "x.jpg", coverJPEG, teacherCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужой id: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Права: 401 без cookie; 403 чужой курс; 403 студент на /teacher/*;
// 404 при удалении несуществующего курса.
func TestCoverAccessErrors(t *testing.T) {
	app, router := newTestApp()
	app.Storage = &fakeCoverStore{}
	ownerCookies, courseID := seedTeacherCourse(t, app, router, "owner")

	// 401: запрос без cookie.
	rec := postCover(t, router, courseID, "x.jpg", coverJPEG, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 403: другой учитель мешает на чужом курсе (и загрузкой, и удалением).
	otherCookies, _ := seedTeacherCourse(t, app, router, "other")
	rec = postCover(t, router, courseID, "x.jpg", coverJPEG, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой загрузка: status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/cover", nil, otherCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("чужой удаление: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 403: студент вообще не входит в /teacher/*.
	studentCookies, _ := registerUser(t, router, "cover-student@example.com", "password123", "")
	rec = postCover(t, router, courseID, "x.jpg", coverJPEG, studentCookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("студент: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 404: удаление обложки несуществующего курса.
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/00000000-0000-0000-0000-000000000000/cover", nil, ownerCookies)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("удаление несуществующего курса: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Архивный курс заморожен: и загрузка, и удаление отвечают 409.
func TestCoverArchivedCourse(t *testing.T) {
	app, router := newTestApp()
	covers := &fakeCoverStore{}
	app.Storage = covers
	teacherCookies, courseID := seedTeacherCourse(t, app, router, "archive")

	rec := doRequest(t, router, http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/archive", map[string]any{}, teacherCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = postCover(t, router, courseID, "x.jpg", coverJPEG, teacherCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("upload в архиве: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var errBody map[string]any
	decodeBody(t, rec, &errBody)
	if errBody["error"] != "Курс в архиве: обложка заморожена" {
		t.Fatalf("текст 409=%v", errBody["error"])
	}
	if len(covers.files) != 0 {
		t.Fatalf("файл записан в архив (не должен): %v", covers.files)
	}
	rec = doRequest(t, router, http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/cover", nil, teacherCookies)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete в архиве: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Разовая страховка сериализации: coverPath отсутствует пустым плохим
// значением (""), а только null/строкой.
func TestCoverPathNeverEmptyString(t *testing.T) {
	app, router := newTestApp()
	app.Storage = &fakeCoverStore{}
	teacherCookies, courseID := seedTeacherCourse(t, app, router, "noempty")

	rec := doRequest(t, router, http.MethodGet, "/api/v1/teacher/courses/"+courseID, nil, teacherCookies)
	var raw map[string]json.RawMessage
	decodeBody(t, rec, &raw)
	course := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw["course"], &course); err != nil {
		t.Fatalf("decode course: %v", err)
	}
	rawCover, ok := course["coverPath"]
	if !ok {
		t.Fatal("курс без coverPath (контракт: string|null)")
	}
	var cover any
	if err := json.Unmarshal(rawCover, &cover); err != nil {
		t.Fatalf("decode coverPath: %v", err)
	}
	if cover != nil {
		t.Fatalf("у нового курса coverPath=%v, ждали null", cover)
	}
}
