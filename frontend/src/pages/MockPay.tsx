import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { confirmMockPayment } from '../api/client';
import { t } from '../i18n/t';

// Демо-страница mock-оплаты: провайдер mock редиректит сюда.
// Реальных денег нет — кнопка подтверждает платёж через webhook.
export default function MockPay(): JSX.Element {
  const { providerPaymentId = '' } = useParams();
  const [done, setDone] = useState(false);
  const mutation = useMutation({
    mutationFn: () => confirmMockPayment(providerPaymentId),
    onSuccess: () => setDone(true),
  });

  return (
    <div className="wrap">
      <div className="page" style={{ maxWidth: 560, margin: '0 auto' }}>
        <div className="crumbs">
          <Link to="/">Каталог</Link> / <span>Оплата</span>
        </div>
        <section className="panel">
          {!done ? (
            <>
              <span className="pill">Демо-режим · mock</span>
              <h1 style={{ margin: '12px 0 8px', fontSize: 28 }}>Тестовая оплата</h1>
              <p className="muted">
                Это страница демо-провайдера: реальные деньги не спишутся. Нажмите кнопку,
                чтобы подтвердить платёж и открыть доступ к курсу.
              </p>
              <p className="muted" style={{ fontSize: 13 }}>
                Платёж: <code>{providerPaymentId || '—'}</code>
              </p>
              <button
                type="button"
                className="btn btn-primary"
                disabled={!providerPaymentId || mutation.isPending}
                onClick={() => mutation.mutate()}
              >
                {mutation.isPending ? 'Подтверждаем…' : 'Оплатить'}
              </button>
              {mutation.isError && (
                <p role="alert" className="error" style={{ marginTop: 12 }}>
                  {t('common.error')}
                </p>
              )}
            </>
          ) : (
            <>
              <span className="pill" style={{ background: 'var(--ok-bg)', color: '#14663A' }}>
                Оплачено
              </span>
              <h1 style={{ margin: '12px 0 8px', fontSize: 28 }}>Доступ открыт</h1>
              <p className="muted">Курс уже в «Моих курсах» — можно начинать учиться.</p>
              <Link className="btn btn-primary" to="/me">
                Перейти к обучению
              </Link>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
