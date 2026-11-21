// Local test-only SMTP sink. Never imported by the application or production build.
import net from 'node:net';
import http from 'node:http';

const host = process.env.MAIL_TEST_HOST ?? '127.0.0.1';
const messages = [];
const smtp = net.createServer(socket => {
  let buffer = '', data = false, body = [], recipients = [];
  socket.setEncoding('utf8'); socket.setTimeout(10_000, () => socket.destroy());
  socket.write('220 local-test-mail ESMTP\r\n');
  socket.on('error', () => {});
  socket.on('data', chunk => {
    buffer += chunk;
    if (buffer.length > 256 * 1024) { socket.destroy(); return; }
    let end;
    while ((end = buffer.indexOf('\r\n')) >= 0) {
      const line = buffer.slice(0, end); buffer = buffer.slice(end + 2);
      if (data) {
        if (line === '.') {
          messages.push({ to: [...recipients], raw: body.join('\n') });
          if (messages.length > 1000) messages.shift();
          data = false; body = []; recipients = []; socket.write('250 queued\r\n');
        } else { body.push(line.startsWith('..') ? line.slice(1) : line); }
      } else if (/^(EHLO|HELO) /i.test(line)) socket.write('250 local-test-mail\r\n');
      else if (/^MAIL FROM:/i.test(line)) { recipients = []; socket.write('250 OK\r\n'); }
      else if (/^RCPT TO:/i.test(line)) { recipients.push(line.match(/<([^>]+)>/)?.[1]?.toLowerCase() ?? ''); socket.write('250 OK\r\n'); }
      else if (/^DATA$/i.test(line)) { data = true; body = []; socket.write('354 End with dot\r\n'); }
      else if (/^QUIT$/i.test(line)) socket.end('221 bye\r\n');
      else if (/^(RSET|NOOP)/i.test(line)) socket.write('250 OK\r\n');
      else socket.write('502 unsupported\r\n');
    }
  });
});
const api = http.createServer((request, response) => {
  const url = new URL(request.url, 'http://localhost');
  response.setHeader('Content-Type', 'application/json');
  response.setHeader('Cache-Control', 'no-store');
  if (url.pathname === '/') {
    response.setHeader('Content-Type', 'text/html; charset=utf-8');
    response.end(`<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Тестовая почта Edu Platform</title>
<style>body{font:16px system-ui;max-width:960px;margin:32px auto;padding:0 16px}article{border:1px solid #ddd;border-radius:12px;padding:16px;margin:16px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere}a{display:block;margin:8px 0;overflow-wrap:anywhere}</style>
<h1>Локальная тестовая почта</h1><p>Письма не отправляются наружу и хранятся только до перезапуска сервиса.</p><button id="refresh">Обновить</button><p id="status" role="status"></p><main id="messages"></main>
<script>
async function refresh(){try{const result=await fetch('/messages');const data=await result.json();const root=document.getElementById('messages');root.replaceChildren();for(const message of data.items.slice().reverse()){const article=document.createElement('article');const title=document.createElement('strong');title.textContent='Кому: '+message.to.join(', ');article.append(title);const pre=document.createElement('pre');pre.textContent=message.raw;article.append(pre);for(const value of message.raw.match(/https?:\\/\\/[^\\s<>]+/g)||[]){const link=document.createElement('a');link.href=value;link.textContent='Открыть ссылку из письма';link.target='_blank';link.rel='noopener noreferrer';article.append(link)}root.append(article)}document.getElementById('status').textContent='Писем: '+data.items.length}catch{document.getElementById('status').textContent='Не удалось загрузить письма'}}
document.getElementById('refresh').addEventListener('click',refresh);refresh();
</script></html>`);
  } else if (url.pathname === '/healthz') response.end('{"ok":true}');
  else if (request.method === 'GET' && url.pathname === '/messages') {
    const to = url.searchParams.get('to')?.toLowerCase();
    response.end(JSON.stringify({ items: messages.filter(message => !to || message.to.includes(to)) }));
  } else { response.statusCode = 404; response.end('{}'); }
});
smtp.listen(11025, host);
api.listen(13025, host);
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { smtp.close(); api.close(); });
