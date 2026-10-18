package handler

// T41: транслитерация кириллицы для slug.
//
// Открытая находка T20: slugRe пропускал только латиницу, поэтому кириллический
// slug отклонялся 400, а авто-slug course-<hex8> был нечитаем в URL каталога.
// Контракт Task/00-api-contract.md («Slug: транслитерация кириллицы (T41)»):
// сначала нижний регистр, кириллица — по таблице, всё не из [a-z0-9] — дефис,
// повторы дефисов схлопываются, дефисы по краям обрезаются, длина до 120.

import (
	"strconv"
	"strings"
)

// Лимиты slug — те же, что в slugRe (^[a-z0-9-]{3,120}$), см. teacher.go.
const (
	slugMinLen = 3
	slugMaxLen = 120
)

// translitRunes — таблица транслитерации кириллицы в латиницу (T41, из контракта).
// ъ и ь дают пустую строку, остальные — латинские буквы/буквосочетания.
var translitRunes = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d",
	'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n",
	'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch",
	'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "yu", 'я': "ya",
}

// translitSlug переводит произвольную строку в slug: нижний регистр,
// кириллица — по таблице translitRunes, всё остальное не из [a-z0-9] — дефис;
// повторы дефисов схлопываются, дефисы по краям обрезаются, длина — до 120.
// Результат может быть короче 3 символов или пустым: вызывающий сам решает,
// что это значит (авто-slug — откат на course-<hex8>, кастомный — 400).
func translitSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false // последний записанный символ — дефис (или начало строки)
	for _, r := range s {
		rep, ok := translitRunes[r]
		if !ok {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
				rep = string(r)
			default:
				rep = "-"
			}
		}
		switch {
		case rep == "":
			// ъ/ь не дают символа; на соседние дефисы не влияют.
		case rep == "-":
			// Дубли и ведущие дефисы пропускаем.
			if b.Len() == 0 || prevDash {
				continue
			}
			prevDash = true
			b.WriteByte('-')
		default:
			prevDash = false
			b.WriteString(rep)
		}
	}
	// Все символы латинские (однобайтовые), поэтому len == длина в рунах,
	// и обрезка по байтам безопасна.
	out := strings.TrimRight(b.String(), "-")
	if len(out) > slugMaxLen {
		// После обрезки дефис может оказаться на конце — снимаем его,
		// чтобы результат по-прежнему проходил slugRe.
		out = strings.TrimRight(out[:slugMaxLen], "-")
	}
	return out
}

// slugFromTitle строит авто-slug из названия курса. Если после транслитерации
// осталось меньше 3 символов — возвращает пустую строку: решение о fallback
// на course-<hex8> принимает вызывающий (makeSlug).
func slugFromTitle(title string) string {
	s := translitSlug(title)
	if len(s) < slugMinLen {
		return ""
	}
	return s
}

// slugWithSuffix добавляет номер к основе авто-slug («-2»…«-9» для занятых),
// укладываясь в лимит 120 символов: при нехватке места основу обрезаем по
// границе дефиса (обрезанная основа гарантированно длиннее 3 символов —
// slugFromTitle отбрасывает короткие).
func slugWithSuffix(base string, n int) string {
	suffix := "-" + strconv.Itoa(n)
	if len(base)+len(suffix) > slugMaxLen {
		base = strings.TrimRight(base[:slugMaxLen-len(suffix)], "-")
	}
	return base + suffix
}
