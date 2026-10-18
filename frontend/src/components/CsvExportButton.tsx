// T40: кнопка выгрузки списка админки в CSV (без сторонних библиотек).
// Преобразование данных в CSV-текст вынесено в чистую экспортируемую функцию
// toCsv, проверка усечения выборки — в exportTruncated (без DOM) — их смогут
// покрыть тесты; компонент отвечает только за состояние кнопки и скачивание
// файла через Blob + временную ссылку.
import { useState } from 'react';
import { ApiError } from '../api/client';
import { t } from '../i18n/t';
import '../styles/csvExport.css';

/** Значение ячейки CSV: null/undefined превращается в пустое поле. */
export type CsvCell = string | number | null;

/** Сообщение под кнопкой: тон (стиль) и текст из словаря. */
type ExportMessage = { tone: 'ok' | 'warn' | 'error' | 'empty'; text: string };

/** Пропсы кнопки выгрузки. */
type CsvExportButtonProps = {
  /** Имя скачиваемого файла, например users-2026-10-18.csv. */
  filename: string;
  /** Заголовки CSV (первая строка файла). */
  headers: string[];
  /**
   * Загрузка всей выборки по текущим фильтрам. Пустой набор строк — выгружать
   * нечего: файл не создаётся, показывается admin.exportEmpty. Флаг truncated
   * (выборка усечена потолком выгрузки) — показывается admin.exportTruncated.
   */
  loadRows: () => Promise<{ rows: Array<Array<CsvCell>>; truncated: boolean }>;
  /** Внешняя блокировка кнопки (например, список ещё не загружен). */
  disabled?: boolean;
};

/** Ячейка в CSV: каждое поле в кавычках, внутренние кавычки удваиваются. */
function escapeCell(value: CsvCell | undefined): string {
  const s = value == null ? '' : String(value);
  return `"${s.replace(/"/g, '""')}"`;
}

/**
 * Собрать CSV-текст: BOM (чтобы Excel открыл UTF-8 корректно), строка
 * заголовков, строки данных. Разделитель — «;», строки разделяются CRLF,
 * переводы строк внутри значений сохраняются (поле в кавычках).
 * Чистая функция без DOM — принимает данные, возвращает строку.
 */
export function toCsv(headers: string[], rows: Array<Array<CsvCell>>): string {
  const lines: string[] = [headers.map((h) => escapeCell(h)).join(';')];
  for (const row of rows) {
    lines.push(row.map((cell) => escapeCell(cell)).join(';'));
  }
  return `\uFEFF${lines.join('\r\n')}`;
}

/**
 * Выборка усечена потолком выгрузки (T43): загружено меньше, чем всего
 * подходит под фильтры. Чистая функция без DOM — по образцу toCsv; страницы
 * передают total первой страницы пагинации и число фактически загруженных
 * строк.
 */
export function exportTruncated(total: number, loaded: number): boolean {
  return total > loaded;
}

/** Дата для имени файла: ГГГГ-ММ-ДД (локальная дата пользователя). */
export function csvDateStamp(): string {
  const d = new Date();
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Скачать текст файлом: Blob + временная ObjectURL-ссылка (без библиотек). */
function downloadCsv(filename: string, text: string): void {
  const blob = new Blob([text], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  // Ссылку добавляем в документ: без этого Firefox может проигнорировать клик.
  document.body.appendChild(link);
  link.click();
  link.remove();
  // Ревокация после клика; отдельный тик — чтобы скачивание успело начаться.
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** Текст ошибки без краха страницы (схема из AdminUsers/AdminCourses). */
function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Кнопка «Выгрузить CSV»: «идёт выгрузка / успех / предупреждение / ошибка / пусто». */
export default function CsvExportButton({
  filename,
  headers,
  loadRows,
  disabled,
}: CsvExportButtonProps): JSX.Element {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<ExportMessage | null>(null);

  async function onExport(): Promise<void> {
    setBusy(true);
    setMsg(null);
    try {
      const { rows, truncated } = await loadRows();
      if (rows.length === 0) {
        // Файл не создаём: выгружать нечего.
        setMsg({ tone: 'empty', text: t('admin.exportEmpty') });
        return;
      }
      downloadCsv(filename, toCsv(headers, rows));
      if (truncated) {
        // Файл создан, но выборка усечена потолком выгрузки — честно сообщаем.
        setMsg({ tone: 'warn', text: t('admin.exportTruncated') });
        return;
      }
      setMsg({ tone: 'ok', text: t('admin.exported') });
    } catch (e: unknown) {
      setMsg({ tone: 'error', text: errorText(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="ce-export">
      <button
        type="button"
        className="btn"
        disabled={disabled || busy}
        title={t('admin.exportCsvHint')}
        onClick={() => void onExport()}
      >
        {t('admin.exportCsv')}
      </button>
      {msg && (
        <p
          className={`ce-msg ce-msg--${msg.tone}`}
          role={msg.tone === 'error' ? 'alert' : 'status'}
        >
          {msg.text}
        </p>
      )}
    </div>
  );
}
