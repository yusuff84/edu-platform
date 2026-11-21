// Read only the rendered Stepik pages in a browser profile logged in by its owner.
// No direct API calls, answer submissions, or access to hidden answer keys.
const { createRequire } = require('node:module');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const requireFrontend = createRequire(path.resolve(__dirname, '../frontend/package.json'));
const { chromium } = requireFrontend('@playwright/test');

const courseId = '146168';
const output = process.env.STEPIK_EXPORT_DIR || path.resolve(__dirname, '../.local/stepik-' + courseId);
const profile = process.env.STEPIK_BROWSER_PROFILE || path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), '.local/share'), 'CodexStepikBrowser');
const startUrl = 'https://stepik.org/lesson/910172/step/1?unit=915779';
const writeJSON = async (name, data) => {
  const target = path.join(output, name);
  await fs.writeFile(target + '.tmp', JSON.stringify(data, null, 2) + '\n');
  await fs.rename(target + '.tmp', target);
};

async function navigate(page, url) {
  let error;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60000 });
      await ready(page);
      return;
    } catch (e) { error = e; console.log('NAVIGATION_RETRY', attempt + 1, new URL(url).pathname); }
  }
  throw error;
}

async function ready(page) {
  await page.locator('.step-show .step-view').waitFor({ timeout: 90000 });
  if (await page.locator('.step-show .quiz-show').count()) {
    await page.locator('.step-show .quiz-component, .step-show .quiz-plugin__content input, .step-show .quiz-plugin__content textarea').first().waitFor({ timeout: 60000 });
  }
  await page.waitForTimeout(700);
}

async function extract(page) {
  return page.evaluate(() => {
    const article = document.querySelector('.step-show');
    const clean = element => {
      const clone = element.cloneNode(true);
      clone.querySelectorAll('script,style,input,button,textarea,[class*="discussion"],.attempt-wrapper-footer,.attempt-wrapper-buttons,.print-only').forEach(n => n.remove());
      for (const node of [clone, ...clone.querySelectorAll('*')]) {
        for (const attr of [...node.attributes]) {
          if (/^on/i.test(attr.name) || /^(id|name|data-ember.*|data-processed)$/.test(attr.name)) node.removeAttribute(attr.name);
        }
        for (const attr of ['src', 'href', 'poster']) {
          const value = node.getAttribute(attr);
          if (!value) continue;
          const url = new URL(value, location.href);
          if (!['https:', 'http:'].includes(url.protocol)) node.removeAttribute(attr);
          else node.setAttribute(attr, url.href);
        }
      }
      return clone.innerHTML;
    };
    const contents = [...article.querySelectorAll('.html-content')];
    const video = article.querySelector('video');
    const quiz = article.querySelector('.quiz-component');
    const pin = document.querySelector('.m-step-pin__link.active');
    return {
      sourceUrl: location.href,
      title: document.querySelector('.top-tools__lesson-name')?.innerText.trim() || document.title,
      stepNumber: Number(location.pathname.match(/\/step\/(\d+)/)?.[1]),
      stepKind: video ? 'video' : quiz ? (quiz.dataset.type || 'quiz') : 'text',
      html: contents.map(clean).join('\n'),
      text: contents.map(e => e.innerText).join('\n\n'),
      exercise: article.querySelector('.quiz-show') ? {
        instruction: article.querySelector('.attempt-wrapper__typename')?.innerText.trim() || '',
        type: quiz?.dataset.type || null,
        multiple: !!article.querySelector('input[type="checkbox"]'),
        options: [...article.querySelectorAll('.choice-quiz-show__option')].map(e => ({ text: e.innerText, html: clean(e) })),
        contentHtml: [...article.querySelectorAll('.quiz-plugin__content')].map(clean).join('\n'),
        answerKey: null,
      } : null,
      videos: [...new Set([...article.querySelectorAll('video,video source')].flatMap(e => [e.currentSrc, e.src]).filter(Boolean))],
      posters: [...article.querySelectorAll('video[poster]')].map(e => e.poster),
      images: [...article.querySelectorAll('.html-content img,.quiz-plugin__content img')].map(e => ({ url: e.currentSrc || e.src, alt: e.alt })),
      attachments: [...article.querySelectorAll('.html-content a[href]')].map(e => ({ url: e.href, title: e.innerText })),
      embeds: [...article.querySelectorAll('iframe[src]')].map(e => e.src),
      pinTitle: pin?.getAttribute('title') || '',
      capturedAt: new Date().toISOString(),
    };
  });
}

async function main() {
  if (process.argv.includes('--login')) {
    const browser = await chromium.launchPersistentContext(profile, { headless: false, viewport: null });
    const page = browser.pages()[0] || await browser.newPage();
    await page.goto(startUrl, { waitUntil: 'domcontentloaded', timeout: 60000 });
    console.log('Log in on Stepik, then close this browser window.');
    await new Promise(resolve => browser.once('close', resolve));
    return;
  }
  await fs.mkdir(path.join(output, 'steps'), { recursive: true });
  const previous = await fs.readFile(path.join(output, 'manifest.json'), 'utf8').then(JSON.parse).catch(() => null);
  const completed = new Map((previous?.modules || []).flatMap(m => m.lessons).filter(l => l.steps?.length && !(previous.errors || []).some(e => e.lessonId === l.id)).map(l => [l.id, l]));
  const browser = await chromium.launchPersistentContext(profile, { headless: true });
  const errors = [];
  try {
    const page = browser.pages()[0] || await browser.newPage();
    await navigate(page, startUrl);
    await page.waitForFunction(() => document.querySelectorAll('.sidebar-module-header').length === 9, null, { timeout: 60000 });
    const sections = await page.locator('.sidebar-module-header').evaluateAll(es => es.map(e => ({ id: e.dataset.section, title: e.innerText.trim().replace(/\s+/g, ' ') })));
    const modules = sections.map(section => ({ ...section, lessons: [] }));
    // The lesson sidebar omits some offscreen links. The syllabus loads all lesson
    // titles as they enter the viewport, so scroll it before reading the outline.
    await page.goto('https://stepik.org/course/' + courseId + '/syllabus', { waitUntil: 'domcontentloaded', timeout: 60000 });
    await page.locator('.lesson-widget__title-text').first().waitFor({ timeout: 60000 });
    for (let i = 0; i < 15; i++) {
      await page.mouse.wheel(0, 650);
      await page.waitForTimeout(350);
    }
    await page.waitForFunction(() => document.querySelectorAll('.lesson-widget__title-text').length === 23, null, { timeout: 60000 });
    const lessons = await page.locator('.lesson-widget__title-text').evaluateAll(es => es.map(e => ({ title: e.innerText.trim().replace(/\s+/g, ' '), url: e.href, id: e.pathname.match(/\/lesson\/(\d+)/)[1] })));
    for (const lesson of lessons) {
      const moduleNumber = Number(lesson.title.match(/^(\d+)\./)?.[1]);
      if (!modules[moduleNumber - 1]) throw new Error('Unrecognized syllabus lesson position');
      modules[moduleNumber - 1].lessons.push(lesson);
    }
    const manifest = {
      courseId, title: 'Цифровая гигиена', sourceUrl: 'https://stepik.org/course/' + courseId,
      method: 'Rendered browser DOM; no direct API requests',
      attribution: 'Авторы: Александр Галимов, Илья Костенецкий. Дальневосточный федеральный университет. Материалы опубликованы на Stepik.',
      license: 'Check the course license and preserve attribution before redistribution.',
      expectedStepCount: null, expectedLessonCount: 23, capturedAt: new Date().toISOString(), modules,
      limitations: ['Hidden answer keys are unavailable in student pages.', 'Video and image URLs are references until downloaded; they may expire.'],
    };
    await writeJSON('manifest.json', manifest);
    console.log('OUTLINE', modules.length, 'modules', modules.reduce((n, m) => n + m.lessons.length, 0), 'lessons');
    for (const module of modules) for (const lesson of module.lessons) {
      try {
        const cached = completed.get(lesson.id);
        if (cached) {
          let valid = true;
          for (const ref of cached.steps) {
            const data = await fs.readFile(path.join(output, ref.file), 'utf8').then(JSON.parse).catch(() => null);
            if (!data?.capturedAt || data.stepNumber !== ref.number) valid = false;
          }
          if (valid) {
            lesson.steps = cached.steps;
            lesson.expectedStepCount = cached.expectedStepCount || cached.steps.length;
            console.log('CACHED_LESSON', lesson.id, cached.steps.length);
            continue;
          }
        }
        const first = new URL(lesson.url);
        first.pathname = '/lesson/' + lesson.id + '/step/1';
        await navigate(page, first.href);
        const links = [...new Set(await page.locator('.m-step-pin__link').evaluateAll(es => es.map(e => e.href)))];
        if (!links.length) throw new Error('No rendered step navigation');
        lesson.expectedStepCount = links.length;
        lesson.steps = [];
        for (const url of links) {
          const number = Number(new URL(url).pathname.match(/\/step\/(\d+)/)[1]);
          const file = 'steps/' + lesson.id + '-' + number + '.json';
          try {
            const cached = JSON.parse(await fs.readFile(path.join(output, file), 'utf8'));
            if (cached.sourceUrl && cached.capturedAt) {
              lesson.steps.push({ number, file, kind: cached.stepKind });
              console.log('CACHED', lesson.id, number);
              continue;
            }
          } catch {}
          if (new URL(page.url()).pathname !== new URL(url).pathname) {
            await page.locator('.m-step-pin__link[href*="/step/' + number + '?"]').first().click();
            await page.waitForURL(u => u.pathname === new URL(url).pathname, { timeout: 30000 });
            await ready(page);
          }
          const data = await extract(page);
          if (data.stepNumber !== number) throw new Error('Step navigation mismatch');
          if (!data.text && !data.videos.length && !data.exercise && !data.embeds.length) throw new Error('Empty rendered material');
          await writeJSON(file, data);
          lesson.steps.push({ number, file, kind: data.stepKind });
          console.log('SAVED', lesson.id, number, data.stepKind, data.text.length);
        }
      } catch (error) {
        errors.push({ lessonId: lesson.id, message: error.message });
        console.log('LESSON_ERROR', lesson.id, error.message.split('\n')[0]);
      }
      manifest.errors = errors;
      await writeJSON('manifest.json', manifest);
    }
    const steps = modules.flatMap(m => m.lessons.flatMap(l => l.steps || []));
    manifest.expectedStepCount = modules.reduce((n, m) => n + m.lessons.reduce((s, l) => s + (l.expectedStepCount || 0), 0), 0);
    manifest.summary = { modules: modules.length, lessons: modules.reduce((n,m) => n + m.lessons.length,0), steps: steps.length, errors: errors.length };
    await writeJSON('manifest.json', manifest);
    console.log('SUMMARY', JSON.stringify(manifest.summary));
    if (errors.length || steps.length !== manifest.expectedStepCount || manifest.summary.lessons !== manifest.expectedLessonCount) process.exitCode = 1;
  } finally {
    await browser.close();
  }
}
main().catch(e => { console.error(e.message); process.exitCode = 1; });
