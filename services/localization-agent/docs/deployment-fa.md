# استقرار واقعی — ۲۰۲۶-۱۰-۲۱

> به‌روزرسانی انتشار: همین درس با درخواست صریح کاربر منتشر شد؛ فرانت‌اند و نگاشت‌های چندزبانهٔ اپلیکیشن تغییر کردند. شرح نصب اولیهٔ زیر تاریخی است؛ [گزارش جاری انتشار](../reports/publication-fa.md) را ببینید.

سرویس فعال: **https://leksuu.ir/localization/mcp**. این endpoint برای MCP است؛ درخواست
بدون توکن عمداً 401 دریافت می‌کند. کلید OpenAI فقط در کنترل‌گر قرار دارد و در فرایند MCP خالی است.

## سورس و اجرای مستقل

سورس در `services/localization-agent` مخزن `amanc1361/leksuu-backend` و شاخهٔ
`feature/educational-localization` قرار دارد. سرویس Python، lockfile، Docker، schema و
ابزار کنترل مستقل‌اند. کدهای Go و APIهای برنامه تغییر نکرده‌اند؛ فایل `.dockerignore`
بک‌اند فقط برای خارج‌کردن این سرویس از build context برنامهٔ Go اضافه شده است.
مسیر قبلی محلی localization-agent به مسیر جدید symlink شده تا فایل‌لینک‌ها کار کنند.

روی سرور:

- سورس: `/opt/leksuu/localization-agent`
- کانتینر: `leksuu-localization-mcp-1`
- image: `leksuu-localization:0.1.5`
- image ID: `sha256:3fabf6ac739199a47e6764ec7c1c4aef73da57fe198f4ae3075121663126025f`
- شبکهٔ داخلی موجود: `platform`؛ هیچ پورت عمومی جدید یا PostgreSQL منتشر نشده است.
- کاربر کانتینر: 10001، فایل‌سیستم فقط‌خواندنی، وضعیت health: healthy.
- gateway: `/opt/platform/edge/conf.d/leksuu.caddy`، مسیر `/localization/*` به سرویس مستقل.
- فایل‌های secret: `/opt/platform/secrets/localization.env` و
  `/opt/platform/secrets/localization-control.env` با مالک root و مجوز 0600.

MCP فقط کلید اتصال نقش محدود و توکن اختصاصی MCP دارد؛ OpenAI key در MCP خالی است.
کلید کنترل‌گر طبق دستور صریح کاربر تنظیم شده؛ مدل پیش‌فرض gpt-6.1-sol و سقف مجاز 5 است.
هر دو مدل Sol/Astra واقعاً در Agents اجرا شدند. worker دائمی یا اجرای کل دوره فعال نیست.

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

- ۳۶ آزمون محلی PostgreSQL/MCP موفق؛ یک هشدار deprecation مربوط به TestClient.
- روی سرور، اتصال با localization_service و SELECT پنج جدول آموزشی موفق بود.
- source INSERT/UPDATE/DELETE/TRUNCATE و UPDATE وضعیت human_approved مجاز نیست.
- handshake MCP، فهرست ۵ ابزار و دریافت درس با ۱۰ آیتم، ۸ تمرین و ۱۷ فیلد موفق بود.
- در بررسی اولیه، شمارنده‌های runs/jobs/drafts قبل و بعد صفر بودند؛ پس از پایلوت، یک run، دو job و ۳۴ draft وجود دارد.
- dry-run همان درس روی دیتابیس اصلی: writes=0، billable_requests=0.
- از ماشین بیرون سرور، HTTPS واقعی آزمایش شد: /fa و /api/v1/courses و
  /localization/healthz همگی 200؛ MCP بدون توکن 401؛ initialize/tools/list/tools/call با
  توکن همگی 200 و دریافت ۱۷ فیلد واقعی موفق بود. توکن در خروجی ثبت نشد.
- config کامل Caddy قبل از reload معتبر بود؛ سرویس‌های اپلیکیشن restart نشدند.

پس از این بررسی‌های اولیه، اتصال از خود OpenAI و inference هر دو مدل تأیید شد.
پایلوت یک درس completed است؛ ۱۷ فارسی و ۱۷ سورانی، همگی نیازمند بازبینی انسانی.
مصرف معلوم تقریباً ۰٫۴۹ دلار است؛ usage دو نشست ناموفق هنوز نامعلوم است.
کیفیت سورانی تأیید بومی نشده؛ گزارش واقعی در ../reports/pilot-result-fa.md است.

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

کلید فعلی در OPENAI_API_KEY فایل امن localization-control.env تنظیم شده است.
برای تغییر تنظیمات از ویرایشگر امن استفاده کنید و مقدار کلید را در فرمان shell، commit
یا log ننویسید. دسترسی مدل را با check-openai بررسی کنید.

```bash
docker compose run --rm controller l10n check-openai
```

پایلوت واقعی تمام شده؛ ترجمهٔ کل دوره و انتشار خودکار فعال نشده‌اند.
شناسهٔ اجرا: `94d21562-0c7e-4d76-9017-dd0bc4ec4ef9`.
نسخهٔ 0.1.5 اصلاح کنترل‌گر روی همان وابستگی‌های قفل‌شدهٔ image اولیه است؛
فایل Dockerfile کامل نیز برای build مستقل در مخزن وجود دارد.

## توقف و rollback سرویس

```bash
cd /opt/leksuu/localization-agent
docker compose stop mcp
```

برای rollback gateway، فایل backup بالا را به مسیر اصلی برگردانید، config کامل Caddy را
validate و سپس reload کنید. حذف schema با sql/001_down.sql تمام پیش‌نویس‌های جدید را
پاک می‌کند؛ فقط پس از backup و تصمیم صریح حذف داده اجرا شود. برای توقف سرویس نیازی به
حذف schema یا تغییر داده‌های اپلیکیشن نیست.
