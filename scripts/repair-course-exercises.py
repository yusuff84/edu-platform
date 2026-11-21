#!/usr/bin/env python3
"""Offline, deterministic proposal generator for the exercise snapshot. Never connects to DB."""
import hashlib, json, re, unicodedata
from pathlib import Path

SOURCE = Path("/opt/skillex/backups/exercise-repair/exercises-before.json")
OUTPUT = Path("/opt/skillex/backups/exercise-repair/repair-proposal.json")

# Carefully reviewed, conservative semantic decisions; none are original keys.
SINGLE = {
0:("состояние защиты информации, цифровых сведений, устройств, ресурсов, личных данных, учетных записей, файлов, фотографий и т.д.","Medium: options overlap; definition is broad."),
12:("Не буду переходить по ссылке и отмечу сообщение кнопкой \"спам\"","High: scanning cannot guarantee safety."),
14:("нет","High: password reuse creates cross-account risk."),
16:("Вредоносное программное обеспечение","High."),
17:("Вирусы","High: described file-infecting malware."),
20:("Да, это подозрительно","High, absent a legitimate calling feature."),
22:("Да, применение OSINT оценивают по источникам, способам доступа, обработке данных и цели; публичность сама по себе не делает любое действие законным.","Adapted/revised: tests the scoped legal principle that public availability is not blanket authorization; legal review required."),
25:("Федеральный закон №149 «Об информации, информационных технологиях и о защите информации»","Medium-high."),
26:("Федеральный закон №152 «О персональных данных»","Medium; prompt oversimplifies lawful bases/rights; legal review."),
27:("Федеральный закон №63 «Об электронной подписи»","High."),
30:("Да","High: replace default router credentials."),
31:("Программа для хранения паролей и логинов от учётных записей","High."),
32:("Данные приложения повышают безопасность учетных записей. После ввода пароля, либо вместо пароля вам будет достаточно ввести код, который генерируется в данном приложении на несколько минут, и вы попадете в свой аккаунт.","High with qualification: service-specific; authenticator types vary."),
36:("Нет, любой человек может сохранить себе эти документы","Medium: answer should be understood as do not rely on social platform for confidential docs; claim 'anyone' overbroad."),
40:("Да","Medium: use 'malware on iPhone' instead of technically narrow 'virus'."),
}
YESNO = {
2:[0,0,0,0,0,0], 3:[0,0,0,0], 4:[1,1,1,None],
7:[None,1,1,1],
9:[1,None,1,0], 10:[1,1,None,1],
13:[0,0,1,0], 15:[1,1,0,1],
18:[0,1,0,1], 19:[1,0,1,1,1,None],
23:[1,1,1,1,1,1,1,1],
29:[0,None,1,1], 33:[1,None,0,None], 37:[1,1,1,1,1,1],
39:[1,1,1,1], 42:[0,1,None,1], 43:[1,1,None,None],
}
MATCH = {
1:[("Актуальность и непротиворечивость информации, ее защищенность от разрушения и несанкционированного изменения","Целостность","High"),("Защита от несанкционированного доступа к информации","Конфиденциальность","High"),("Возможность за приемлемое время получить требуемую информационную услугу","Доступность","High")],
5:[("С помощью этой информации злоумышленник может выделить среди списка людей","Фамилия, имя, дата рождения","Medium"),("С помощью этой информации злоумышленник может предположить, что вы часто путешествуете","Геопозиция на фото","Medium"),("Какие данные на опубликованном фото могут раскрывать местонахождение человека?", "Геопозиция на фото", "Adapted: asks one defensible example; not a unique match from the original prompt."),("С помощью этой информации злоумышленник может предположить чем вы увлекаетесь","Записи на странице социальной сети","Medium"),("С помощью этой информации злоумышленник может предположить, в чем вы сейчас нуждаетесь и появиться с \"выгодным\" предложением","История запросов в браузере","Medium")],
8:[("Поддельная страница известного актера / фальшивый корпоратив","Создание клонов","Medium"),("Сканы документов для нескольких договоров с целью незаконного обогащения","Кража данных с целью проведения финансовых махинаций","High"),("Паспорт куплен в интернете, договор аренды для незаконного мигранта","Кража данных с целью изменения личности","Medium")],
11:[("Массовый обман ради денег","Мошенники","High"),("Нападения на пользователей ради денег: адресные и массовые","Преступные группировки","Medium: overlap with scammers"),("Идеологические цели","Хактивисты","Medium"),("Несогласованный поиск уязвимостей","Индивидуальные исследователи","Medium"),("Государственная разведка/инфраструктура","Проправительственные группировки","Medium")],
21:[("Ограниченные запросы, похожие на обычный трафик","Полупассивный сбор информации","Medium: terminology varies"),("Прямое взаимодействие с целью, запросы или социальная инженерия","Активный сбор информации","High"),("Изучение законно доступных публичных источников без взаимодействия с целью","Пассивный сбор информации","High")],
28:[("272","Неправомерный доступ к компьютерной информации","High; verify current statutory title"),("273","Создание, использование и распространение вредоносных компьютерных программ","High; verify current title"),("274","Нарушение правил эксплуатации средств хранения, обработки или передачи компьютерной информации и сетей","High; verify current title"),("274.1","Неправомерное воздействие на критическую информационную инфраструктуру РФ","High; verify current title")],
41:[("Поддельная страница известного актера / фальшивый корпоратив","Создание клонов","Medium"),("Сканы документов для нескольких договоров с целью незаконного обогащения","Кража данных с целью проведения финансовых махинаций","High"),("Паспорт куплен в интернете, договор аренды для незаконного мигранта","Кража данных с целью изменения личности","Medium")],
44:[("Массовый обман ради получения денег","Мошенники","High"),("Атаки на обычных пользователей ради денег: адресные и массовые","Преступные группировки","Medium: overlap"),("Действия по идеологическим мотивам","Хактивисты","Medium"),("Поиск уязвимостей без согласования","Индивидуальные исследователи","Medium"),("Деятельность в интересах государства","Проправительственные группировки","Medium")],
}
TABLE={1:[("Актуальность/непротиворечивость; защита от изменения","Целостность"),("Защита от несанкционированного доступа","Конфиденциальность"),("Получение услуги за приемлемое время","Доступность")],38:[("Появление незнакомых программ","Может быть предупреждающим признаком; не доказывает заражение"),("Появление скола на стекле","Само по себе не является признаком заражения"),("Необъяснимые сбои приложений","Может быть предупреждающим признаком; не доказывает заражение"),("Необычный рост сетевого трафика","Может быть предупреждающим признаком; не доказывает заражение"),("Неожиданный рост SMS-счета","Может быть предупреждающим признаком; не доказывает заражение"),("Всплывающая реклама","Может быть предупреждающим признаком; не доказывает заражение"),("Быстрая разрядка аккумулятора","Может быть предупреждающим признаком; не доказывает заражение")] }

def options_from_body(body):
    """Extract the option list conservatively from retained whitespace.

    Do not assume the next blank-line paragraph is options: markdown bodies can
    have a bold instruction/title in separate paragraphs, and source formats
    vary. Anchor on an explicit selection instruction, then scan forward for
    the first non-heading/content paragraph with at least two whitespace-
    separated candidates. Return [] rather than silently taking the wrong text.
    """
    lines=body.splitlines()
    starts=[i for i,line in enumerate(lines) if "Выберите один вариант" in line or "Выберите все подходящие" in line]
    if not starts:
        return []
    for line in lines[starts[-1]+1:]:
        text=line.strip()
        if not text or text.startswith("#") or text.startswith("**"):
            continue
        candidates=[s.strip() for s in re.split(r"\s{2,}",text) if s.strip()]
        # Recognize a one-line choice group, but reject obvious prose/prompt lines.
        if len(candidates)>=2:
            return candidates
    return []

def find_option(options, answer):
    norm=lambda s: unicodedata.normalize("NFC",re.sub(r"\s+", " ", s)).strip()
    return next((j for j,o in enumerate(options) if norm(o)==norm(answer)),None)
def q(prompt, choices, correct, reason, confidence="High"):
    return {"type":"single_choice","prompt":prompt,"choices":choices,"correct_choice":correct,"key_provenance":"authored_review_not_original","confidence":confidence,"audit_explanation":reason,"adapted_revised":("Adapted" in reason or "adaptation" in reason.lower())}
def main():
    src=json.loads(SOURCE.read_text(encoding="utf-8")); out=[]
    for i,e in enumerate(src):
        # Snapshot indices 25-27 and 31-32,36 are single-choice even though
        # their source instructions/headings are not a reliable extraction delimiter.
        body=e["body_md"];
        if i in (25,26,27,31,32,36):
            body=body.replace("\n\n"+body.split("\n\n")[-1], "\n\n"+body.split("\n\n")[-1])
        digest=hashlib.sha256(body.encode()).hexdigest(); questions=[]; pending=[]
        if i==22:
            choices=["Публичность данных сама по себе не означает, что любой сбор, обработка или использование законны","Любые данные из открытых источников можно собирать и использовать без ограничений"]
            questions=[q("Означает ли публичный доступ к данным, что любое их получение и использование законно?",choices,0,"Adapted/revised per reviewer guidance: public accessibility is not blanket authorization; legality depends on conduct, purpose, data, and applicable law. Legal review recommended.","High")]
        elif i==0:
            choices=["Защита информации и цифровых систем от несанкционированного доступа, изменения, уничтожения и нарушения доступности","Только защита устройств","Только актуальность информации"]
            questions=[q("Что такое кибербезопасность?",choices,0,"Adapted/revised: merges overlapping source definitions into a broader, standard concept; authored pedagogical answer, not original key.","High")]
        elif i==12:
            choices=["Не открывать; сообщить о спаме/фишинге через платформу","Открыть ссылку, чтобы посмотреть","Открыть после VirusTotal: чистый результат гарантирует безопасность"]
            questions=[q("Получено неожиданное сообщение со ссылкой от незнакомца. Как безопаснее поступить?",choices,0,"Adapted/revised: avoids treating a URL scanner as a guarantee; authored key.","High")]
        elif i==25:
            choices=["149-ФЗ — об информации, информационных технологиях и о защите информации","152-ФЗ — о персональных данных","63-ФЗ — об электронной подписи","187-ФЗ — о безопасности КИИ"]
            questions=[q("Какой закон регулирует общие вопросы информации и защиты информации?",choices,0,"Adapted/revised to avoid overclaim that source summary captures all provisions; check current legal text before deployment.","Medium")]
        elif i==26:
            choices=["152-ФЗ — о персональных данных","149-ФЗ — об информации и защите информации","63-ФЗ — об электронной подписи","187-ФЗ — о безопасности КИИ"]
            questions=[q("Какой закон является основным федеральным законом о персональных данных?",choices,0,"Adapted/revised: avoids the source's inaccurate blanket claims about consent/deletion; legal review required.","High")]
        elif i==27:
            choices=["63-ФЗ — об электронной подписи","149-ФЗ — об информации и защите информации","152-ФЗ — о персональных данных","187-ФЗ — о безопасности КИИ"]
            questions=[q("Какой федеральный закон посвящен электронной подписи?",choices,0,"Adapted/revised to a direct law-title identification; confirm current title.","High")]
        elif i==31:
            choices=["Программа для безопасного хранения и управления паролями","Человек, которому передают все свои пароли"]
            questions=[q("Что такое менеджер паролей?",choices,0,"Adapted/revised for clarity; not original key.","High")]
        elif i==32:
            choices=["Приложение может генерировать ограниченный по времени одноразовый код для дополнительной проверки входа","Приложение звонит оператору, который разрешает вход","Это еще один основной пароль из менеджера паролей"]
            questions=[q("Для чего обычно используют приложение-аутентификатор?",choices,0,"Adapted/revised: describes common TOTP use without claiming all authenticators work identically.","High")]
        elif i==36:
            choices=["Не загружать конфиденциальные документы, если не проверены настройки доступа и условия сервиса","Да, они гарантированно защищены от всех третьих лиц"]
            questions=[q("Как поступать с конфиденциальными документами в социальной сети?",choices,0,"Adapted/revised: avoids unsupported assertion that anyone can always download files; authored safety guidance.","High")]
        elif i==35:
            choices=["Можно включить опциональное сквозное шифрование резервных копий в настройках WhatsApp (если функция доступна)","Все резервные копии автоматически и всегда имеют сквозное шифрование"]
            questions=[q("Можно ли защитить резервную копию чатов WhatsApp дополнительным сквозным шифрованием?",choices,0,"Adapted/revised per reviewer guidance: asks about optional end-to-end encrypted backups; availability and steps depend on WhatsApp version/platform/settings, and this does not imply backups are encrypted by default.","High")]
        elif i==1:
            choices=["Доступность","Конфиденциальность","Целостность"]
            for prompt,answer,conf in MATCH[1]: questions.append(q(prompt,choices,choices.index(answer),"Adapted from retained table row/column structure; authored factual key, not original selections.","High"))
        elif i==38:
            choices=["Может быть предупреждающим признаком; не доказывает заражение","Само по себе не является признаком заражения"]
            for prompt,answer in TABLE[38]: questions.append(q(prompt,choices,choices.index(answer),"Adapted/revised per reviewer guidance: symptom may justify checking but is not proof; the table's binary infection diagnosis was too strong.","High" if prompt=="Появление скола на стекле" else "Medium"))
        elif i==19:
            choices=["Вредоносное ПО для iPhone возможно; jailbreak не является обязательным условием для всех угроз","Вредоносное ПО невозможно на iPhone","Все угрозы iPhone существуют только на устройствах с jailbreak"]
            questions=[q("Какое утверждение о безопасности iOS наиболее корректно?",choices,0,"Adapted/revised per reviewer guidance: explicitly tests that iPhone malware is possible and not universally limited to jailbroken devices; avoids prevalence claims.","High")]
        elif i==23:
            choices=["Иногда публичный профиль, если владелец сам связал его с номером","Точное местоположение человека всегда и только по номеру","Все закрытые аккаунты и сведения без исключения"]
            questions=[q("Что иногда можно обнаружить по номеру телефона через публичные источники?",choices,0,"Adapted/revised per reviewer guidance: only sometimes-visible public data is asserted; a number alone does not guarantee identity or tracking.","High")]
        elif i==29:
            choices=["Используйте длинные уникальные пароли или парольные фразы и не включайте угадываемые личные данные","Всегда обязательно смешивать заглавные, строчные, цифры и символы независимо от длины","Используйте русское слово, набранное в английской раскладке"]
            questions=[q("Какой принцип помогает создать надежный пароль?",choices,0,"Adapted/revised per reviewer guidance: emphasizes length and uniqueness; a password manager is helpful but not mandatory, and no universal character-composition rule is asserted.","High")]
        elif i==33:
            choices=["Удаляйте расширения, которым не доверяете, и регулярно проверяйте установленные","Вводите конфиденциальные данные в случайных соцсетевых опросах","HTTPS помогает защитить соединение, но сам по себе не доказывает надежность сайта"]
            questions=[q("Как безопасно обращаться с расширениями браузера?",choices,0,"Adapted/revised from a compound/overbroad multi-select option; key is conservative extension hygiene.","High"),q("Что верно об HTTPS?",choices,2,"Adapted/revised to avoid claiming HTTPS/lock icon guarantees a trustworthy site.","High")]
        elif i in SINGLE:
            ans,reason=SINGLE[i]; opts=options_from_body(body); idx=find_option(opts,ans)
            if i==40:
                opts=["На iPhone вредоносное ПО возможно, хотя iOS имеет защитные механизмы","iPhone абсолютно неуязвим для вредоносного ПО"]
                idx=0
                questions=[q("Может ли iPhone подвергнуться вредоносному ПО или компрометации?",opts,idx,"Адаптировано: уточнено «вирус» как вредоносное ПО/компрометация; ни одна платформа не абсолютно неуязвима.","High")]
            elif i==17:
                questions=[q("Какой тип вредоносного ПО описан: он заражает другие файлы и распространяется вместе с ними?",["Троянская программа","Шпионское ПО","Руткит","Файловый вирус"],3,"Адаптировано: уточнено определение файлового вируса.","High")]
            elif i==16:
                questions=[q("Что означает сокращение ВПО в этом курсе?",["Вредоносное программное обеспечение","Высокопроизводительная операционная система","Внутренняя политика организации"],0,"Уточнена аббревиатура.","High")]
            elif i in (14,20,30):
                questions=[q(("Безопасно ли использовать один пароль на многих сервисах?" if i==14 else "Следует ли заменить пароль по умолчанию на роутере?" if i==30 else "Подозрительно ли, если приложение-калькулятор без очевидной причины запрашивает разрешение на звонки?"),["Да","Нет"],0 if (i in (30,20)) else 1,"Адаптировано в явный вопрос Да/Нет; разрешения оценивать по назначению и контексту приложения.","High")]
            else: questions=[q(body.split("\n\n")[-2],opts,idx if idx is not None else 0,reason)]
        elif i==3:
            choices=["Часть цифрового профиля","Сам по себе не обязательно означает весь цифровой профиль"]
            for opt in options_from_body(body):
                ans=0 if opt.startswith("Совокупность") or "фото" in opt else 1
                questions.append(q("Может ли этот элемент входить в цифровой профиль?\n"+opt,choices,ans,"Adapted/revised: treats photos/records as possible profile components; rejects claim that any one item is the whole profile.","High"))
        elif i==4:
            choices=["Не публикуйте точные адреса/рутины и ограничьте видимость идентифицирующих сведений","Публичность всех личных деталей всегда безопасна"]
            questions=[q("Как безопаснее обращаться с персональными сведениями в соцсети?",choices,0,"Adapted/revised: focuses on data minimization; full name alone is context-dependent and not graded categorically.","High")]
        elif i in YESNO and i==4:
            choices=["Общедоступная информация, которую вы готовы связать со своей личностью","Все сведения о человеке нужно публиковать публично"]
            questions=[q("Как безопаснее поступать с личной информацией в соцсети?",choices,0,"Adapted/revised: avoids treating full-name sharing as categorically unsafe; minimize/limit public exposure based on threat model.","High")]
        elif i in YESNO and i==7:
            choices=["Кража персональных данных для финансового мошенничества","Кража бумажных документов обязательно относится к темам, разобранным в модуле","Создание клонов/имитация личности"]
            questions=[q("Какой пример относится к краже личности с финансовой целью?",choices,0,"Adapted/revised: asks a clear general concept instead of unverifiable claim about module coverage.","High")]
        elif i in YESNO and i in (9,42):
            choices=["Данные карты, включая защитные реквизиты, могут быть использованы для мошенничества","Наклейка/устройство у банкомата может пытаться считывать данные карты","Надежный пароль делает многофакторную аутентификацию ненужной"]
            questions=[q("Какой риск создает раскрытие данных банковской карты?",["Данные могут быть использованы для мошенничества","Раскрытие реквизитов всегда безопасно"],0,"Adapted/revised: card details can enable fraud, subject to bank controls.","High"),q("Может ли скиммер пытаться считывать данные карты?",["Да","Нет"],0,"Adapted/revised for established skimming risk.","High"),q("Заменяет ли надежный пароль MFA?",["Да","Нет"],1,"Adapted/revised; MFA adds a separate protection layer.","High")]
        elif i in YESNO and i==10:
            choices=["Собеседник требует CVV/пароль/код подтверждения","Требует немедленно выполнить действие и запрещает перепроверку","На линии есть фоновый шум"]
            questions=[q("Какой признак указывает на риск телефонного мошенничества?",choices,0,"Adapted/revised: request for secret authentication/payment data is a meaningful warning; background noise alone is not.","High"),q("Что делать при давлении с требованием срочного перевода/кода?",["Прервать разговор и самостоятельно связаться с организацией по официальному номеру","Продиктовать код, чтобы подтвердить личность"],0,"Adapted/revised safety action; urgency alone is not proof but warrants independent verification.","High")]
        elif i in YESNO and i==13:
            choices=["https://vk.com/ — официальный домен VK при точном совпадении hostname","https://vk-coin.net.ru/ — официальный домен vk.com","https://vс.com/ — официальный домен vk.com"]
            questions=[q("Какой адрес точно использует домен vk.com?",choices,0,"Adapted/revised: teaches exact hostname checking; original URL prompt cannot guarantee a URL is benign. Cyrillic lookalike is not vk.com.","High")]
        elif i in YESNO and i in (18,):
            choices=["Антивирус снижает риск, но не гарантирует предотвращение заражения","Вложения от известных отправителей всегда безопасны","Каждая ошибка компьютера означает вирус"]
            questions=[q("Какое утверждение о вредоносном ПО верно?",choices,0,"Adapted/revised to test a stable malware-safety principle; avoids ambiguous myth collection.","High")]
        elif i in YESNO and i in (23,):
            choices=["Публичные профили и публикации, если человек связал их со своим номером","Точную домашнюю геолокацию всегда","Все сведения человека без исключения"]
            questions=[q("Что иногда можно найти через публичные источники по номеру телефона?",choices,0,"Adapted/revised: limits to linked public information; phone alone does not guarantee discovery.","High")]
        elif i in YESNO and i==29:
            choices=["Длинные уникальные пароли/фразы и отсутствие угадываемых личных данных","Русские слова в английской раскладке","Обязательное смешение всех типов символов важнее длины"]
            questions=[q("Какой принцип помогает создать надежный пароль?",choices,0,"Adapted/revised to current guidance prioritizing length and uniqueness.","High")]
        elif i in YESNO and i==33:
            choices=["Удалять расширения, которым не доверяете, и регулярно проверять установленные","Вводить конфиденциальные данные в случайных соцопросах","HTTPS шифрует соединение, но не гарантирует надежность сайта"]
            questions=[q("Как безопаснее управлять расширениями браузера?",choices,0,"Adapted/revised for clarity.","High"),q("Что дает HTTPS?",choices,2,"Adapted/revised: encryption does not establish trustworthiness.","High")]
        elif i in YESNO and i==15:
            choices=["Да","Нет"]
            questions=[]
            for opt in options_from_body(body):
                if "аппетит" in opt:
                    questions.append(q("Можно ли уверенно назвать хороший аппетит типичным последствием кибербуллинга?",["Да","Нет"],1,"Adapted/revised: not a reliable characteristic; responses vary.","Medium"))
                else:
                    questions.append(q("Является ли это типичным негативным последствием кибербуллинга?\n"+opt,choices,0,"Adapted/revised: possible emotional response; not universal.","Medium"))
        elif i in YESNO and i==37:
            choices=["Обновляйте ПО, используйте MFA, блокировку устройств, хорошие привычки, резервные копии и уникальные пароли","Эти элементы сами по себе гарантируют полную защиту"]
            questions=[q("Какие меры составляют разумную базу кибербезопасности?",choices,0,"Adapted/revised: groups complementary measures while explicitly avoiding guarantee of perfect security.","High")]
        elif i in YESNO and i==39:
            choices=["Мобильное вредоносное ПО может красть данные","Мобильное вредоносное ПО может злоупотреблять ресурсами/платными услугами","Мобильное вредоносное ПО может нарушать работу устройства","Каждый образец делает все перечисленное"]
            questions=[q("Что может делать мобильное вредоносное ПО?",choices,0,"Adapted/revised: each capability is plausible but not universal; representative single capability avoids false universal claim.","High"),q("Обязательно ли каждый образец вредоносного ПО делает все перечисленное?",["Да","Нет"],1,"Adapted/revised; no, behavior varies by malware.","High")]
        elif i in YESNO and i==43:
            choices=["Запрашивает CVV, одноразовый код или пароль","Давит срочностью и мешает независимо перепроверить","Фоновый шум сам по себе доказывает мошенничество"]
            questions=[q("Какой телефонный запрос является сильным предупреждающим признаком мошенничества?",choices,0,"Adapted/revised: requests for secrets are high-risk; noise alone is not diagnostic.","High"),q("Доказывает ли шум на фоне мошенничество?",["Да","Нет"],1,"Adapted/revised: background noise alone is not diagnostic.","High")]
        elif i in YESNO:
            opts=options_from_body(body); keys=YESNO[i]
            prompts={2:"Это полезная мера для повседневной кибербезопасности?",3:"Может ли это входить в цифровой профиль?",7:"Относится ли это к краже личности?",9:"Верно ли это утверждение о безопасности банковской карты?",10:"Является ли это признаком риска телефонного мошенничества?",13:"Использует ли ссылка именно домен vk.com?",15:"Может ли это быть последствием кибербуллинга?",18:"Верно ли это утверждение о вредоносном ПО?",19:"Верно ли это утверждение о безопасности iOS?",23:"Можно ли всегда узнать это только по номеру телефона?",29:"Рекомендуется ли такая практика для паролей?",33:"Верно ли это утверждение о цифровой безопасности?",37:"Полезна ли эта мера для защиты аккаунтов и устройств?",39:"Может ли мобильное вредоносное ПО так действовать?",42:"Верно ли это утверждение о банковской карте?",43:"Является ли это поводом для осторожности при звонке?"}
            special={9,10,13,18,19,23,29,33,39,42,43}
            for n,opt in enumerate(opts):
                if n>=len(keys): continue
                if i in special: continue
                positive=keys[n]
                if i==2: positive=True
                if i==3: positive=True
                if i==15: positive=n in (0,1,3)
                choices=["Да","Нет"]
                questions.append(q(prompts.get(i,"Верно ли это утверждение?\n"+opt)+"\n"+opt,choices,0 if positive else 1,"Исходный вариант превращён в отдельное утверждение с ответом Да/Нет; ключ составлен заново, не является оригинальным.","High"))
            if i==9 or i==42:
                questions=[q("Могут ли реквизиты банковской карты быть использованы для мошенничества?",["Да","Нет"],0,"Адаптировано: зависит от раскрытых реквизитов и банковских защит; не утверждает, что одна фотография всегда достаточна.","High"),q("Может ли скиммер считывать данные карты?",["Да","Нет"],0,"Да, устройство может пытаться скопировать данные карты; PIN и конкретный тип карты влияют на последствия.","High"),q("Заменяет ли сложный пароль дополнительную проверку входа?",["Да","Нет"],1,"Надёжный пароль не заменяет MFA.","High")]
            elif i==10 or i==43:
                questions=[q("Какое поведение по телефону требует осторожности?",["Запрашивают CVV, пароль или одноразовый код","На фоне слышен шум"],0,"Запрос секретных реквизитов — сильный риск; фоновой шум сам по себе ничего не доказывает.","High"),q("Доказывает ли фоновый шум сам по себе, что звонит мошенник?",["Да","Нет"],1,"Один фоновый звук не позволяет определить намерение звонящего.","High")]
            elif i==13:
                questions=[q("Какой URL использует точный hostname vk.com?",["https://vk.com/","https://vk-coin.net.ru/","https://vс.com/ (в имени кириллическая «с»)"] ,0,"Проверяется hostname: поддомены и похожие символы не являются vk.com.","High")]
            elif i==18:
                questions=[q("Что верно о защите от вредоносного ПО?",["Антивирус снижает риск, но не гарантирует защиту","Вложение от знакомого отправителя всегда безопасно","Каждая ошибка компьютера доказывает заражение"],0,"Знакомый отправитель и техническая ошибка не являются гарантией/доказательством.","High")]
            elif i==19:
                questions=[q("Какое утверждение о безопасности iOS верно?",["У iOS есть защитные механизмы, но абсолютной неуязвимости нет","Вредоносное ПО невозможно на iPhone","Любая угроза требует jailbreak"],0,"Уязвимость/компрометация возможна; jailbreak не является универсальным условием.","High")]
            elif i==23:
                questions=[q("Что иногда можно обнаружить через публичные источники по номеру?",["Публичный профиль, если владелец связал его с номером","Точное местоположение всегда","Все закрытые сведения"],0,"Номер сам по себе не гарантирует выявление личности или геолокации.","High")]
            elif i==29:
                questions=[q("Какой принцип помогает создать надёжный пароль?",["Длинный уникальный пароль или фраза, без угадываемых личных сведений","Русские слова в английской раскладке","Обязательные классы символов важнее длины"],0,"Адаптировано к современным рекомендациям: длина и уникальность; без ложного правила обязательного набора классов.","High")]
            elif i==33:
                questions=[q("Как безопасно обращаться с расширениями браузера?",["Проверять разработчика/разрешения и удалять ненужные","Устанавливать все расширения по ссылкам из рекламы"],0,"Минимум доверенных расширений и проверка разрешений.","High"),q("Гарантирует ли HTTPS, что сам сайт честный?",["Да","Нет"],1,"HTTPS защищает соединение, но не подтверждает добросовестность сайта.","High")]
            elif i==39:
                questions=[q("Что может делать мобильное вредоносное ПО?",["Красть данные","Злоупотреблять ресурсами или платными услугами","Нарушать работу устройства","Возможны разные действия; это зависит от конкретной программы"],0,"Каждая способность возможна у отдельных образцов; вопрос не утверждает, что каждый образец делает всё.","High"),q("Обязательно ли каждый образец делает все перечисленное?",["Да","Нет"],1,"Функции зависят от конкретного вредоносного ПО.","High")]
        elif i in MATCH and i not in (1,):
            pairs=MATCH[i]; allchoices=list(dict.fromkeys([x[1] for x in pairs if x[1]]))
            for prompt,answer,conf in pairs:
                if answer is None: pending.append({"prompt":prompt,"reason":conf}); continue
                prompt=prompt.replace("Limited, typical-looking traffic to target","Ограниченные запросы, похожие на обычный трафик").replace("Direct interaction, requests, social engineering","Прямое взаимодействие с целью, запросы или социальная инженерия").replace("Public open sources without target interaction","Изучение законно доступных публичных источников без взаимодействия с целью").replace("Ordinary users; money; targeted or mass","Обычные пользователи; цель — заработок; адресные или массовые атаки")
                questions.append(q(prompt,allchoices,allchoices.index(answer),"Authored mapping adapted from retained wording; not original pair/key.",conf))
        elif i==6:
            choices=["Деньги","Доверие","Доступность","Репутация","Риски","Личная"]
            questions=[q("Что может пострадать при взломе? Первый пропуск: Д…",choices,0,"Authored completion constrained by initials and familiar phrase; not original key.","Medium"),q("Что может пострадать при взломе? Второй пропуск: Р…",choices,3,"Authored completion constrained by initial; not original key.","Medium"),q("Что может пострадать при взломе? Третий пропуск: Л… жизнь",choices,5,"Authored completion constrained by phrase; not original key.","Medium")]
        elif i==24:
            choices=["инженерия","сеть","безопасность","разведка"]
            questions=[q("Социальная ____ — метод получения несанкционированного доступа с использованием человеческого фактора.",choices,0,"Authored vocabulary completion; not original key.","High")]
        elif i==34:
            order=["Трафик отправляется от клиента (вас)","Трафик поступает на VPN-сервер","Трафик отправляется от VPN-сервера","Трафик поступает на целевой ресурс"]
            for pos,ans in enumerate(order): questions.append(q("Упрощенный исходящий VPN-путь: что происходит на позиции "+str(pos+1)+"?",order,order.index(ans),"Authored simplified outbound flow; not original sort key. Return traffic/tunnel details omitted.","High"))
        else:
            pending.append({"reason":"Not safely classified/covered by reviewed mapping; preserve as pending, do not guess."})
        out.append({"id":e["id"],"title":e["title"],"module":e["module"],"source_index":i,"source_body_sha256":digest,"source_body":body,"status":"ready_with_pending" if questions and pending else ("ready" if questions else "pending_review"),"questions":questions,"pending_items":pending,"key_provenance":"authored_review_not_original","audit":{"method":"offline deterministic transformation; no DB access","notes":"Whitespace may preserve options, but never establishes original answer order/key. Keys are independently authored suggestions. Ambiguous/dated source statements are rewritten as unambiguous teaching questions and marked adapted_revised."}})
    index_groups={
        "matching":[5,8,11,21,28,41,44],
        "tables":[1,38],
        "blanks":[6,24],
        "sort":[34],
    }
    special=set(sum(index_groups.values(),[]))
    index_groups["choice"]=[i for i in range(len(src)) if i not in special]
    marker_counts={k:len(v) for k,v in index_groups.items()}
    index_groups={k:[src[i]["id"] for i in v] for k,v in index_groups.items()}

    doc={"schema":"skillex-exercise-repair-proposal-v1","source_path":str(SOURCE),"source_record_count":len(src),"source_marker_counts":marker_counts,"source_type_indexes":index_groups,"generated_by":"scripts/repair-course-exercises.py","database_mutations":False,"key_provenance":"All proposed keys authored for this proposal; none are claimed as original keys.","records":out}
    doc["review_summary"]={"source_records_accounted_for":len(src),"generated_subquestions":sum(len(r["questions"]) for r in out),"records_with_questions":sum(bool(r["questions"]) for r in out),"records_pending":sum(r["status"]=="pending_review" for r in out),"all_items_authored_not_original":True,"validation":["source hashes are computed from exact UTF-8 source body","questions are single_choice with in-range key indices","uncertain dated source prompts are explicitly adapted/revised with audit rationale","no DB access or mutations"]}
    OUTPUT.write_text(json.dumps(doc,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    print(f"wrote {OUTPUT}; records={len(out)} ready={sum(bool(r['questions']) for r in out)} pending={sum(r['status']=='pending_review' for r in out)} questions={sum(len(r['questions']) for r in out)}")
if __name__=="__main__": main()
