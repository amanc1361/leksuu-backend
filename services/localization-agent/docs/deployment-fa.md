# استقرار واقعی — ۲۰۲۶-۱۰-۲۱

سرویس فعال: **https://leksuu.ir/localization/mcp**. این endpoint برای MCP است؛ درخواست
بدون توکن عمداً 401 دریافت می‌کند. هیچ کلید OpenAI در آن منتشر یا استفاده نشده است.

## سورس و اجرای مستقل

سورس در `services/localization-agent` مخزن `amanc1361/leksuu-backend` و شاخهٔ
`feature/educational-localization` قرار دارد. سرویس Python، lockfile، Docker، schema و
ابزار کنترل مستقل‌اند. کدهای Go و APIهای برنامه تغییر نکرده‌اند؛ فایل `.dockerignore`
بک‌اند فقط برای خارج‌کردن این سرویس از build context برنامهٔ Go اضافه شده است.
مسیر قبلی محلی localization-agent به مسیر جدید symlink شده تا فایل‌لینک‌ها کار کنند.

روی سرور:

- سورس: `/opt/leksuu/localization-agent`
- کانتینر: `leksuu-localization-mcp-1`
- image: `leksuu-localization:0.1.0`
- image ID: `sha256:b087ace496276861d8eaa5e414fa2f29d3d44814d331cb91ac77165e5fa88ff7`
- شبکهٔ داخلی موجود: `platform`؛ هیچ پورت عمومی جدید یا PostgreSQL منتشر نشده است.
- کاربر کانتینر: 10001، فایل‌سیستم فقط‌خواندنی، وضعیت health: healthy.
- gateway: `/opt/platform/edge/conf.d/leksuu.caddy`، مسیر `/localization/*` به سرویس مستقل.
- فایل‌های secret: `/opt/platform/secrets/localization.env` و
  `/opt/platform/secrets/localization-control.env` با مالک root و مجوز 0600.

MCP فقط کلید اتصال نقش محدود و توکن اختصاصی MCP دارد؛ OpenAI key در MCP خالی است.
فایل کنترل‌گر نیز OPENAI_API_KEY و OPENAI_MODEL خالی دارد؛ سقف مجاز پایلوت 5 است.
مدل یا دسترسی حساب حدس زده نشده و worker پولی روشن نیست.

## دیتابیس و بکاپ

schema جدید localization با سه جدول runs/jobs/drafts و تابع ثابت lock_source نصب شد.
نقش localization_service با رمز SCRAM تصادفی مستقل و کمترین مجوز تعریف شد؛ رمز و توکن
فقط در فایل‌های امن سرورند. source tableها و public.content_translations تغییر نکرده‌اند.

قبل از نصب، بکاپ کامل با pg_dump گرفته و با pg_restore --list بررسی شد:

`/root/localization-install-20261021T183433Z/leksuu-before.dump`

بکاپ gateway قبل از تغییر:

`/root/localization-gateway-20261021T183711Z/leksuu.caddy.before`

یک تلاش اولیهٔ نصب در بررسی مجوزها rollback شد. علت، ارزیابی تابع نام جدول قبل از
فیلتر schema در information_schema بود؛ audit به بررسی OID جدول تغییر کرد، آزمون واقعی
PostgreSQL اضافه شد و نصب موفق بعدی کاملاً transactional انجام شد. تلاش ناموفق هیچ
schema/role یا secret نیمه‌نصب‌شده‌ای باقی نگذاشت.

## شواهد واقعی

- ۳۰ آزمون محلی PostgreSQL/MCP موفق؛ یک هشدار deprecation مربوط به TestClient.
- روی سرور، اتصال با localization_service و SELECT پنج جدول آموزشی موفق بود.
- source INSERT/UPDATE/DELETE/TRUNCATE و UPDATE وضعیت human_approved مجاز نیست.
- handshake MCP، فهرست ۵ ابزار و دریافت درس با ۱۰ آیتم، ۸ تمرین و ۱۷ فیلد موفق بود.
- شمارنده‌های runs/jobs/drafts قبل و بعد از بررسی هر سه صفر بودند.
- dry-run همان درس روی دیتابیس اصلی: writes=0، billable_requests=0.
- از ماشین بیرون سرور، HTTPS واقعی آزمایش شد: /fa و /api/v1/courses و
  /localization/healthz همگی 200؛ MCP بدون توکن 401؛ initialize/tools/list/tools/call با
  توکن همگی 200 و دریافت ۱۷ فیلد واقعی موفق بود. توکن در خروجی ثبت نشد.
- config کامل Caddy قبل از reload معتبر بود؛ سرویس‌های اپلیکیشن restart نشدند.

این بررسی‌ها دسترسی خود OpenAI به MCP، دسترسی مدل‌ها یا کیفیت ترجمه را اثبات نمی‌کنند؛
این موارد به کلید جایگزین و پایلوت مجاز وابسته‌اند. هزینهٔ API ایجادشده صفر است.

## کنترل عملیاتی روی سرور

```bash
cd /opt/leksuu/localization-agent
docker compose ps
docker compose logs --tail 30 mcp
docker compose run --rm controller l10n --help
```

برای دریافت وضعیت یک اجرای واقعی بعدی:

```bash
docker compose run --rm controller l10n status RUN_UUID
```

برای OpenAI، کلید افشاشدهٔ قبلی باید در حساب لغو شود. کلید جایگزین را فقط با ویرایشگر
امن در OPENAI_API_KEY فایل localization-control.env قرار دهید؛ مدل را پس از check-openai
انتخاب کنید. مقدار کلید را در چت، فرمان shell، commit یا log ننویسید.

```bash
docker compose run --rm controller l10n check-openai
```

بدون این تنظیمات، ترجمهٔ واقعی شروع نمی‌شود. ترجمهٔ کل دوره و انتشار خودکار فعال نشده‌اند.

## توقف و rollback سرویس

```bash
cd /opt/leksuu/localization-agent
docker compose stop mcp
```

برای rollback gateway، فایل backup بالا را به مسیر اصلی برگردانید، config کامل Caddy را
validate و سپس reload کنید. حذف schema با sql/001_down.sql تمام پیش‌نویس‌های جدید را
پاک می‌کند؛ فقط پس از backup و تصمیم صریح حذف داده اجرا شود. برای توقف سرویس نیازی به
حذف schema یا تغییر داده‌های اپلیکیشن نیست.
