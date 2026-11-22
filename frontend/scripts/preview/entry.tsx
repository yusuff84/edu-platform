// Предпросмотр урока вне сайта: тот же Markdown-компонент и стили, что в обучении.
import { createRoot } from 'react-dom/client';
import { Markdown } from '../../src/components/Markdown';
import '../../src/styles.css';
import '../../src/styles/learn.css';

declare global {
  interface Window { __MD: string; __TITLE: string }
}

createRoot(document.getElementById('root')!).render(
  <div className="study-lesson" style={{ padding: '24px 16px' }}>
    <h1>{window.__TITLE}</h1>
    <div className="lesson-pane">
      <div className="panel">
        <Markdown source={window.__MD} />
      </div>
    </div>
  </div>,
);
