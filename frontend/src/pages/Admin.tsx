// Хаб супер-админки (/admin). Раньше здесь были четыре длинные секции с
// якорями (#queue, #journal, #settings, #invites) — теперь у каждой свой
// отдельный маршрут и своя страница. Хаб только показывает ссылки и не делает
// запросов к API: открытие /admin не должно грузить очередь, журнал, настройки
// и приглашения.
import { Link, Navigate, useLocation } from 'react-router-dom';
import { t } from '../i18n/t';
import type { RuKey } from '../i18n/t';
import { useSeo } from '../app/useSeo';
// Стили страницы изолированы в styles/admin.css — общий styles.css не трогаем.
import '../styles/admin.css';

/**
 * Старые hash-ссылки из закладок и статистики: /admin#queue и т.п.
 * После разделения секций на страницы перенаправляем на новый маршрут,
 * чтобы старые ссылки не открывали пустой хаб с несуществующим якорем.
 */
const HASH_REDIRECTS: Record<string, string> = {
  '#queue': '/admin/moderation',
  '#journal': '/admin/payments',
  '#settings': '/admin/settings',
  '#invites': '/admin/invites',
};

type HubLink = {
  to: string;
  label: RuKey;
};

type HubGroup = {
  /** Подпись группы — короткий русский текст: новых ключей в ru.json не добавляем. */
  title: string;
  links: HubLink[];
};

const HUB_GROUPS: HubGroup[] = [
  {
    title: 'Модерация',
    links: [
      { to: '/admin/moderation', label: 'admin.reviewQueue' },
      { to: '/admin/reviews', label: 'p10.nav.reviews' },
      { to: '/admin/qa', label: 'p10.nav.qa' },
    ],
  },
  {
    title: 'Финансы',
    links: [
      { to: '/admin/payments', label: 'admin.journal' },
      { to: '/admin/promo-codes', label: 'p10.nav.promos' },
    ],
  },
  {
    title: 'Пользователи и курсы',
    links: [
      { to: '/admin/users', label: 'adminUsers.title' },
      { to: '/admin/invites', label: 'admin.invites' },
      { to: '/admin/courses', label: 'adminCourses.title' },
      { to: '/admin/enrollments', label: 'adminEnrollments.title' },
    ],
  },
  {
    title: 'Аналитика и настройки',
    links: [
      { to: '/admin/stats', label: 'adminStats.title' },
      { to: '/admin/settings', label: 'admin.settingsTitle' },
    ],
  },
];

export default function Admin(): JSX.Element {
  const { hash } = useLocation();
  useSeo(`${t('cabinet.adminTitle')} — Edu Platform`);

  // Редирект старых якорей. Navigate с replace не оставляет /admin#... в истории.
  const redirectTo = HASH_REDIRECTS[hash.toLowerCase()];
  if (redirectTo) return <Navigate to={redirectTo} replace />;

  return (
    <div className="wrap">
      <div className="page">
        <main className="admin">
          <h1>{t('cabinet.adminTitle')}</h1>

          <nav className="admin-hub" aria-label={t('p10.nav.aria')}>
            {HUB_GROUPS.map((group) => (
              <section key={group.title} className="admin-hub-group" aria-label={group.title}>
                <h2 className="admin-hub-group__title">{group.title}</h2>
                <div className="admin-hub-cards">
                  {group.links.map((link) => (
                    <Link key={link.to} to={link.to} className="admin-hub-card">
                      <span>{t(link.label)}</span>
                      <span className="admin-hub-card__arrow" aria-hidden="true">
                        →
                      </span>
                    </Link>
                  ))}
                </div>
              </section>
            ))}
          </nav>
        </main>
      </div>
    </div>
  );
}
