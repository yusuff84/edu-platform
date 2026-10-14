import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import { Link, isRouteErrorResponse, useRouteError } from 'react-router-dom';
import { t } from '../i18n/t';
import '../styles/shell.css';

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Классовый React error boundary для поддерева.
 * Перехватывает ошибки рендера (например, урока в /learn) и показывает
 * экран восстановления вместо белого экрана; шапка приложения при этом живёт.
 * Кнопка «Повторить» сбрасывает состояние boundary и пробует отрисовать снова.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Диагностика в консоль: текст и стек компонентов.
    console.error('Ошибка рендера:', error, info.componentStack);
  }

  private onRetry = (): void => {
    this.setState({ error: null });
  };

  render(): ReactNode {
    if (this.state.error) {
      return (
        <div className="shell-center" role="alert">
          <h1>{t('error.render')}</h1>
          <p>{t('error.generic')}</p>
          <p className="muted">{this.state.error.message}</p>
          <div className="shell-actions">
            <button type="button" onClick={this.onRetry}>
              {t('error.retry')}
            </button>
            <Link to="/">{t('error.goCatalog')}</Link>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

/**
 * errorElement для маршрутов react-router (корень и /learn/*).
 * Классовый boundary здесь бесполезен — router передаёт ошибку не через props,
 * поэтому берём её из useRouteError. Ошибка одного урока заменяет только
 * элемент своего маршрута: Layout с навигацией остаётся.
 */
export function RouteError(): JSX.Element {
  const error = useRouteError();
  const detail = isRouteErrorResponse(error)
    ? `${error.status} ${error.statusText}`
    : error instanceof Error
      ? error.message
      : '';
  return (
    <div className="shell-center" role="alert">
      <h1>{t('error.render')}</h1>
      <p>{t('error.generic')}</p>
      {detail && <p className="muted">{detail}</p>}
      <div className="shell-actions">
        <button type="button" onClick={() => window.location.reload()}>
          {t('error.retry')}
        </button>
        <Link to="/">{t('error.goCatalog')}</Link>
      </div>
    </div>
  );
}
