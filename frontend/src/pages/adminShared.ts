// Общие константы, типы и утилиты страниц супер-админки.
// Вынесены из Admin.tsx при разделении разделов на отдельные маршруты
// (/admin/moderation, /admin/payments, /admin/settings, /admin/invites),
// чтобы страницы не дублировали код и не тянули друг друга (циклов нет:
// модуль зависит только от api/i18n, а не от pages/*).
import { ApiError, formatDate, formatMoney } from '../api/client';
import type { Course, PaymentJournalItem, PaymentStatus } from '../api/client';
import type { AdminInviteRow, AdminInviteStatus } from '../api/adminActions';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';

/** Статусы платежей в порядке контракта (00-api-contract.md, US6). */
export const PAYMENT_STATUSES: PaymentStatus[] = [
  'pending',
  'succeeded',
  'failed',
  'refunded',
];

/** Русские подписи статусов — ключи из словаря T02. */
export const STATUS_LABEL: Record<PaymentStatus, RuKey> = {
  pending: 'admin.statusPending',
  succeeded: 'admin.statusSucceeded',
  failed: 'admin.statusFailed',
  refunded: 'admin.statusRefunded',
};

/** Значение limit по умолчанию (контракт: 20, максимум 100). */
export const DEFAULT_LIMIT = 20;

/** Размер страницы приглашений: постановка T51 ограничивает limit диапазоном 10–20. */
export const INVITES_LIMIT = 10;

/** Русские подписи статусов приглашений — ключи из словаря T02. */
export const INVITE_STATUS_LABEL: Record<AdminInviteStatus, RuKey> = {
  sent: 'admin.inviteStatusSent',
  accepted: 'admin.inviteStatusAccepted',
  expired: 'admin.inviteStatusExpired',
  revoked: 'admin.inviteStatusRevoked',
};

/**
 * Курс очереди модерации: аддитивные поля T48 (teacherEmail/modulesCount/
 * lessonsCount) появятся после его выхода — до тех пор их может не быть,
 * поэтому типизируем как опциональные и не падаем при отсутствии.
 */
export type QueueCourse = Course & {
  teacherEmail?: string;
  modulesCount?: number;
  lessonsCount?: number;
};

/** Размер страницы выгрузки CSV: контракт разрешает limit до 100. */
export const EXPORT_PAGE_SIZE = 100;

/** Предохранитель выгрузки: не больше 50 страниц = 5000 строк (см. отчёт T38). */
export const EXPORT_MAX_PAGES = 50;

export function isPaymentStatus(value: string | null): value is PaymentStatus {
  return value !== null && (PAYMENT_STATUSES as readonly string[]).includes(value);
}

/** Прошла ли дата; некорректную дату считаем «не истекло» — статус не подменяем. */
export function isInviteExpired(expiresAt: string): boolean {
  const time = new Date(expiresAt).getTime();
  return !Number.isNaN(time) && time < Date.now();
}

/**
 * Отображаемый статус приглашения: сырой `sent` с истёкшим expiresAt
 * показываем как «Истекло» (бэкенд статус не пересчитывает — контракт T47).
 * Сырой `expired` из хранилища тоже выводим как «Истекло».
 */
export function inviteDisplayStatus(
  item: Pick<AdminInviteRow, 'status' | 'expiresAt'>,
): AdminInviteStatus {
  if (item.status === 'sent' && isInviteExpired(item.expiresAt)) return 'expired';
  return item.status;
}

/** Короткий идентификатор для таблицы. */
export function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

/** Текст ошибки без краха страницы: 403/401/400/сеть/прочее. */
export function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t('cabinet.needRole');
    if (error.status === 401) return t('error.needLogin');
    if (error.status === 400) return error.message || t('common.error');
    return error.message || t('common.error');
  }
  if (error instanceof TypeError) return t('error.network');
  return t('common.error');
}

/** Копирование в буфер обмена: Clipboard API, иначе запасной способ. */
export async function copyToClipboard(value: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Нет прав/небезопасный контекст — пробуем запасной способ.
  }
  try {
    const area = document.createElement('textarea');
    area.value = value;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    area.remove();
    return ok;
  } catch {
    return false;
  }
}

// ---------- Выгрузка журнала в CSV ----------

/** Сегодняшняя локальная дата как ГГГГ-ММ-ДД — для имени файла выгрузки. */
export function todayIsoDate(): string {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}

/** Ячейка CSV: значение всегда в кавычках, внутренние кавычки удваиваются. */
export function csvCell(value: string): string {
  return `"${value.replace(/"/g, '""')}"`;
}

/**
 * CSV выборки журнала: разделитель «;» (русский Excel), заголовки — из словаря,
 * порядок колонок совпадает с таблицей на странице.
 */
export function paymentsCsv(rows: PaymentJournalItem[]): string {
  const header = [
    t('admin.colDate'),
    t('admin.colStudent'),
    t('admin.colCourse'),
    t('admin.colAmount'),
    t('admin.colCommission'),
    t('admin.colTeacher'),
    t('admin.colProvider'),
    t('admin.colStatus'),
    t('admin.colPaymentId'),
  ];
  const lines = rows.map((item) => [
    formatDate(item.createdAt),
    // Как в таблице: имя → email → короткий id.
    item.studentName || item.studentEmail || shortId(item.studentId),
    item.courseTitle,
    formatMoney(item.amountCents),
    formatMoney(item.platformFeeCents),
    formatMoney(item.teacherAmountCents),
    item.provider,
    t(STATUS_LABEL[item.status]),
    item.providerPaymentId,
  ]);
  // CRLF — привычный для Excel разделитель строк.
  return [header, ...lines].map((cells) => cells.map(csvCell).join(';')).join('\r\n');
}

/** Скачивание текста как файла; CSV собирается вручную через Blob (новых зависимостей нет). */
export function downloadTextFile(fileName: string, content: string, mimeType: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: mimeType }));
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
