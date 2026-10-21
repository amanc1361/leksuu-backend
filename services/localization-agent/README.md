# سرویس مستقل ترجمهٔ آموزشی Leksuu

پیش‌نویس فارسی و سورانی برای درس‌های انگلیسی PostgreSQL، با اجنت میزبانی‌شدهٔ OpenAI
و MCP مستقل. کد و APIهای بک‌اند و فرانت‌اند اپلیکیشن تغییر نکرده‌اند. انتشار خودکار وجود ندارد.

مسیر سورس: `leksuu-backend/services/localization-agent`، در شاخهٔ مستقل مخزن موجود بک‌اند.
مسیر قبلی `localization-agent` یک symlink برای سازگاری فایل‌لینک‌های قبلی است.

## وضعیت تحویل

سرویس در سرور مستقر است و migration مستقل پس از درخواست ادامهٔ استقرار اعمال شد.
HTTPS، MCP احراز هویت‌شده، دسترسی محدود دیتابیس و dry-run واقعی تأیید شده‌اند.
پایلوت پولی و مقایسهٔ مدل‌ها در انتظار کلید امن جایگزین هستند؛ دسترسی حساب OpenAI
هنوز تأیید نشده است. سقف ۵ دلار ثبت شده و هیچ هزینهٔ API ایجاد نشده است.

- [گزارش استقرار واقعی و دستورهای سرور](docs/deployment-fa.md)
- [طراحی، نگاشت داده و محدودیت‌ها](docs/architecture-fa.md)
- [SQL دقیق و تأیید استقرار](docs/production-approval-fa.md)
- [گزارش تست و پایلوت](reports/verification-fa.md)
- [جدول بازبینی انگلیسی/فارسی/سورانی](reports/pilot-review.html)؛ ترجمه‌ها هنوز اجرا نشده‌اند.

## اجرای آزمایشی بدون کلید، دیتابیس یا هزینه

Python 3.12 و uv لازم است. از ریشهٔ همین پروژه:

```bash
uv sync --frozen
uv run --frozen l10n dry-run --fixture tests/fixtures/real-lesson.json
uv run --frozen python scripts/test-postgres.py
```

دستور تست، PostgreSQL 17 موقت با نام تصادفی روی loopback ایجاد و در پایان حذف می‌کند.
به دیتابیس اصلی وصل نمی‌شود. در نخستین dry-run، tiktoken فایل tokenizer عمومی را دانلود و cache می‌کند؛ سپس
نیازی به OpenAI یا درخواست پولی ندارد. اجرای مستقیم pytest بدون متغیر تست، آزمون‌های دیتابیس را
skip می‌کند؛ برای پوشش کامل همان اسکریپت بالا را اجرا کنید.

`start` بدون `--execute` هم فقط dry-run است. dry-run هیچ run/job/draft و migration
ایجاد نمی‌کند. اتصال به درس واقعی نیز فقط SELECT است:

```bash
l10n dry-run --lesson LESSON_UUID
```

## تنظیمات امن

کلید افشاشده در گفتگو باید در OpenAI لغو شود. آن را در این پروژه وارد نکنید.
فایل نمونه فقط placeholder دارد. فایل واقعی را با chmod 600، بیرون از git نگه دارید.
سرویس MCP فقط به LOCALIZATION_DATABASE_URL با نقش محدود، MCP_BEARER_TOKEN و
MCP_ALLOWED_HOST نیاز دارد؛ کلید OpenAI را فقط کنترل‌گر داشته باشد.

فایل کنترل‌گر علاوه بر این‌ها OPENAI_API_KEY، MCP_PUBLIC_URL، OPENAI_MODEL و
APPROVED_BUDGET_USD را دارد. مقدار بودجه برای این پایلوت 5 است؛ این مقدار را به‌عنوان
مجوز تازه برای اجرای نامحدود تلقی نکنید. رزرو بودجهٔ runها در دیتابیس جمع می‌شود و
مجموع نمی‌تواند از این مجوز عبور کند؛ رزرو اجراهای failed خودکار آزاد نمی‌شود. برای تولید MCP token از `openssl rand -hex 32`
استفاده کنید و خروجی را مستقیماً در secret file ذخیره کنید، نه در چت/گزارش.

بارگذاری محلی فایل امن با shell (فایل باید فقط شامل تنظیمات مورد اعتماد شما باشد):

```bash
set -a
. /ABSOLUTE/SECURE/PATH/localization-control.env
set +a
l10n check-openai
```

بررسی دسترسی فقط GET /models و GET /agents/sessions انجام می‌دهد و inference ندارد.
اگر 403/404 رخ داد، دسترسی Agents/مجوزهای پروژه را بررسی کنید؛ سیستم به اجرای محلی
تغییر نمی‌کند. مجوزهای لازم طبق quickstart رسمی: api.agents.read، api.agents.write و
api.responses.write. وجود مدل در فهرست، تضمین فعال‌بودن inference نیست.

## دیتابیس و استقرار واقعی

ابتدا [درخواست تأیید دقیق](docs/production-approval-fa.md) را بخوانید. SQL با حساب DBA
اجرا می‌شود؛ حساب runtime حق migration ندارد. ترتیب: 001_up.sql، سپس 002_role.sql،
تنظیم رمز با `\password localization_service`، سپس 003_audit.sql. بررسی کنید نقش
دیگری/PUBLIC به این حساب دسترسی نوشتن مبدأ نداده باشد.

در نصب فعلی، فایل‌های پروژه در `/opt/leksuu/localization-agent` بگذارید.
secretهای مستقل پیشنهادی:

- `/opt/platform/secrets/localization.env`: اتصال نقش محدود + MCP token + allowed host.
- `/opt/platform/secrets/localization-control.env`: همان اتصال/توکن + کلید OpenAI و بودجه/مدل.

PostgreSQL و سرویس MCP روی شبکهٔ داخلی موجود platform می‌مانند. Compose پورت عمومی
ندارد. برای آزمایش محلی MCP بدون Docker از `l10n serve` استفاده کنید؛ آن را مستقیماً
روی اینترنت منتشر نکنید.

```bash
docker compose build
docker compose up -d mcp
docker compose ps
```

اگر DNS شبکهٔ build محیط شما به PyPI دسترسی ندارد، proxyهای موجود میزبان را بدون
نوشتن مقدارشان در Dockerfile منتقل کنید:

```bash
docker compose build --build-arg HTTP_PROXY --build-arg HTTPS_PROXY --build-arg NO_PROXY
```

نسخهٔ پیشنهادی gateway در config/leksuu.caddy.proposed از فایل واقعی خوانده‌شده ساخته
شده است. بعد از بازبینی/backup و `caddy validate` جایگزین و reload شود؛ سرورهای دیگر
Caddy را تغییر ندهید. مسیر مستقل بدون DNS جدید:

`MCP_PUBLIC_URL=https://leksuu.ir/localization/mcp` و `MCP_ALLOWED_HOST=leksuu.ir`.

نمونهٔ بررسی قبل از reload:

```bash
docker exec platform-caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
```

پس از reload، health محلی صرفاً وضعیت HTTP process را نشان می‌دهد؛ سلامت DB/OpenAI
نیست. روی URL عمومی یک درخواست بدون توکن باید 401 بدهد. سپس handshake MCP با توکن و
فراخوانی get_rules/get_lesson_context را آزمایش کنید؛ پاسخ سایت 200 به‌تنهایی کافی نیست.

## پایلوت یک درس؛ ترجمه و بازبینی

اول dry-run و نرخ روز OpenAI را بررسی کنید. همواره یک مدل صریح از نتیجه check-openai
انتخاب کنید. نمونهٔ زیر فقط اگر دسترسی مدل واقعاً تأیید شده باشد معتبر است:

```bash
l10n start --lesson LESSON_UUID --model gpt-6.1-sol --budget-usd 5 --execute --accept-soft-budget
l10n work RUN_UUID --max-jobs 2
l10n status RUN_UUID
l10n export RUN_UUID > /SECURE/PATH/drafts.json
python scripts/review-report.py tests/fixtures/real-lesson.json /SECURE/PATH/review.html --drafts /SECURE/PATH/drafts.json
```

برای report از snapshot همان درس و نسخهٔ اجرا استفاده کنید. فایل fixture همراه پروژه
فقط برای همان درس نمونه است. خروجی‌ها متن آموزشی هستند؛ آن‌ها را در مسیر امن نگه دارید.
در Docker کنترل‌گر را با volume پایدار اجرا کنید:

```bash
docker compose run --rm controller l10n check-openai
docker compose run --rm controller l10n status RUN_UUID
```

هیچ دستور واقعی «ترجمهٔ کل دوره» فعال نشده است. انتشار و اجرای گسترده پس از تأیید
پایلوت و نمونه‌های سورانی نیازمند مرحلهٔ جداگانه‌اند.

## توقف، ادامه و تغییر مبدأ

```bash
l10n pause RUN_UUID
l10n resume RUN_UUID --max-jobs 2
l10n refresh RUN_UUID
```

pause اجرای تازه را می‌بندد و نشست‌های فعال شناخته‌شده را cancel می‌کند. لغو فوری و
توقف آنی هزینه تضمین نمی‌شود. claim پس از حدود ۱۲۰ ثانیه قابل بازیابی است؛ سپس همان
نشست لغوشده با کلید idempotency ادامه می‌یابد. نشست‌های خطادار قطعی خودکار دوباره
ایجاد نمی‌شوند. حداکثر ۳ claim و ۲ ادامهٔ لغوشده در هر phase مجاز است.

برای علامت‌گذاری ترجمه‌های قدیمی، `refresh` را دوره‌ای با همان نقش محدود اجرا کنید.
`export` هم refresh می‌کند. این دو دستور برخلاف dry-run، needs_review را می‌نویسند؛ متن
یا وضعیت تأیید انسانی را عوض نمی‌کنند. تغییری در جداول مبدأ برای trigger داده نشده است.

## خطاها

- `openai_http_401/403`: کلید جایگزین و مجوزهای Agents را کنترل کنید؛ متن کلید را چاپ نکنید.
- `openai_http_404`: دسترسی حساب/endpoint رسمی را بررسی کنید؛ fallback محلی انجام نمی‌شود.
- `submission_outcome_unknown_reconcile_before_retry`: درخواست شاید ساخته شده باشد.
  فایل 0600 مربوطه در volume state و نشست‌های OpenAI را تطبیق دهید. فایل submitting
  را بی‌بررسی حذف نکنید؛ این کار ممکن است هزینهٔ تکراری ایجاد کند. اگر نشست پیدا شد،
  session_id همان نشست را در state ثبت کنید؛ کلید یا payload محرمانه در آن نگذارید.
- `source_changed` / `stale`: dry-run تازه و enqueue نسخهٔ جدید؛ ترجمهٔ قدیمی روی جدید ثبت نمی‌شود.
- `claim_expired`: worker قبلی متوقف شده یا کند است؛ پس از lease بازیابی کنید.
- `remote_turn_failed` یا `validation_or_database_failed`: همان مورد failed مانده است؛
  خروجی خصوصی و نسخهٔ مبدأ را بررسی کنید. صف دوباره از ابتدا اجرا نمی‌شود.
- `phase_budget_threshold_reached`: نشست لغو شده؛ مجوز بودجه را خودکار افزایش ندهید.
- هزینهٔ polling best-effort است؛ اگر سقف سخت ۵ دلار الزام شماست، گزینهٔ accept-soft-budget
  را نزنید تا محدودیت واقعی سمت ارائه‌دهنده بررسی شود.

## rollback

ابتدا MCP و کنترل‌گر را متوقف و مسیر جدید gateway را به backup برگردانید. از schema
localization با pg_dump نسخهٔ پشتیبان بگیرید؛ secret اتصال را در خروجی نمایش ندهید.
001_down.sql فقط سه جدول جدید و تابع قفل را حذف می‌کند؛ تمام پیش‌نویس‌ها حذف می‌شوند.
جدول‌ها/متن‌های اپلیکیشن دست‌نخورده‌اند. برای حذف نقش پس از حذف schema، DBA مجوزهای
SELECT مبدأ را با `DROP OWNED BY localization_service` در همین دیتابیس لغو و سپس
`DROP ROLE localization_service` اجرا کند. نقش را در دیتابیس‌های دیگر مشترک نکنید.
