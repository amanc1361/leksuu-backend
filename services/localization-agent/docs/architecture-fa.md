# طراحی و شواهد محیط — ۲۰۲۶-۱۰-۲۱

این پروژه مستقل است. هیچ فایل Go/Next.js، API یا منطق بک‌اند تغییر نکرده است.
سورس در services/localization-agent مخزن leksuu-backend، روی شاخه feature/educational-localization نگهداری می‌شود. ریشهٔ leksuu خودش مخزن Git نیست؛ سرویس Python و استقرار آن مستقل از برنامهٔ Go است.

## محیط مشاهده‌شده

- سیستم توسعه Linux 7.0، Python 3.12، Docker 29.1.3، psql موجود.
- سرور با SSH واقعی بررسی شد: Linux 6.8.0، Docker 29.1.3، PostgreSQL 17 داخل شبکه platform.
- PostgreSQL پورت منتشرشده ندارد؛ سرویس جدید هم هیچ پورت دیتابیس را منتشر نمی‌کند.
- ۱۹ دوره، ۳۰۵۳ درس، ۳۲۳۷۳ تمرین. این شمارش همه زبان‌هاست و برآورد حجم ترجمه انگلیسی نیست.
- ساختار واقعی public و نمونه یک درس کامل با ۱۰ آیتم و ۸ تمرین خوانده شد؛ بررسی اولیه READ ONLY بود؛ در مرحلهٔ استقرار بعدی فقط schema مستقل localization نصب شد. جزئیات در deployment-fa.md است.
- از هر ۹ نوع تمرین واقعی یک نمونه برای آزمون و ارزیابی نگهداری شده است.
- Caddy مرکزی: `/opt/platform/edge/conf.d/leksuu.caddy`؛ نام کانتینر platform-caddy.
- تنظیمات فعلی اپلیکیشن در `/opt/platform/secrets/leksuu.env` باقی می‌ماند. مقادیر محرمانه ثبت نشده‌اند.

## مسیر رسمی OpenAI

مستندات جاری، Agents API را با هدر `OpenAI-Beta: agents=v1` و مسیر
`POST /v1/agents/sessions` معرفی می‌کنند. agent loop، ابزارخوانی و وضعیت نشست در OpenAI
مدیریت می‌شود. این پروژه فقط کنترل شروع/پیگیری/لغو و ذخیرهٔ نتیجه را اجرا می‌کند.
`environment.type=none` یعنی اجنت محیط shell ندارد، ولی اجرای اجنت همچنان در OpenAI است.
MCP با transport HTTP و `connection_origin=service` مستقیماً از OpenAI فراخوانی می‌شود.
نیازی به sandbox پولی یا دادن SQL/SSH/فایل‌های سرور به اجنت نیست.

Agent Builder ابزار بصری متفاوتی است. مستندات فعلی پایان آن را برای ۳۰ نوامبر ۲۰۲۶
اعلام کرده‌اند؛ پایهٔ این پیاده‌سازی نیست. اجرای محلی Agents SDK نیز انتخاب نشده است.
هیچ fallback خودکار به اجنت محلی وجود ندارد. اگر حساب Agents API را نداشته باشد، ادامهٔ
معماری نیازمند تصمیم کاربر است.

قابلیت حساب هنوز تأیید نشده: کلید امن جایگزین دریافت نشده است. نام‌های رسمی
`gpt-6-astra` و `gpt-6.1-sol` در مستندات مشاهده شدند؛ دسترسی حساب فقط با GET /models
و دسترسی خواندن Agents بررسی می‌شود. دسترسی نوشتن و inference با پایلوت بودجه‌دار روشن می‌شود.

منابع رسمی خوانده‌شده:

- https://developers.openai.com/api/docs/guides/agents-api/overview
- https://developers.openai.com/api/docs/guides/agents-api/quickstart
- https://developers.openai.com/api/docs/guides/agents-api/tools/mcp
- https://developers.openai.com/api/docs/guides/agents-api/sessions
- https://developers.openai.com/api/docs/guides/agent-builder
- https://developers.openai.com/api/docs/models/gpt-6-astra
- https://developers.openai.com/api/docs/models/gpt-6.1-sol
- https://developers.openai.com/api/docs/pricing
- https://developers.openai.com/api/reference/resources/beta/subresources/agents/subresources/sessions/methods/create

## نگاشت دادهٔ واقعی

روابط: courses → units → lessons → lesson_items → exercises.
lesson_items.ref_id و media_key همراه payload برای زمینه خوانده می‌شوند؛ هیچ تغییری ندارند.
جدول‌های رسانه/واژگان جداگانه خارج از این adapter هستند؛ آیتم‌هایی که زمینهٔ کاملشان
در این پنج جدول نیست، نیازمند بررسی و گسترش adapter مستقل‌اند.

| مبدأ | رفتار |
|---|---|
| lessons.title و objective.summary.en / points.*.en / tip.en | پیش‌نویس ترجمهٔ رابط درس |
| grammar payload.title و payload.rule.en | پیش‌نویس؛ حفظ literalهای انگلیسی |
| text payload.phrase | حفظ اصل؛ تولید معنی در مسیر مجزای payload.translation |
| exercises.prompt.instruction و explanation یا explanation.en | پیش‌نویس ترجمه |
| prompt.question | فقط دستورهای مشخص مانند Choose/Select و بدون جای خالی؛ باقی موارد حفظ و علامت بررسی |
| true_false.prompt.statement | حفظ؛ چون بخشی از ارزیابی است و نگاشت انسانی لازم دارد |
| answer_key، options، words، segments، left/right، text، hint، ترتیب | حفظ کامل؛ در allowlist نیستند |
| grammar.forms، subject و form، audio، image، video، شناسه‌ها | حفظ کامل؛ برچسب‌های مبهم نیز خودکار ترجمه نمی‌شوند |
| reading، roleplay، writing، dialogue و انواع ناشناخته | حفظ؛ بررسی دستی adapter |

مثال واقعی: `You ____ English well.` ترجمه نمی‌شود؛ گزینه‌های speak/speaks و correct=1
نیز تغییر نمی‌کنند. توضیح دلیل استفاده از speak قابل ترجمه است. از ۱۷ فیلد مجاز درس
نمونه، چهار مورد ارزیابی برای تصمیم نگاشت انسانی علامت خورده‌اند.

## نگهداری و کنترل دسترسی

فقط schema جدید localization با runs/jobs/drafts ساخته می‌شود. جدول موجود
public.content_translations متصل به انتشار اپلیکیشن است؛ این سرویس به آن دسترسی ندارد.
ترجمهٔ جدید در اپلیکیشن دیده نخواهد شد تا مرحلهٔ انتشار جداگانه طراحی و تأیید شود.

هش JSON canonical از کل زمینه شامل پاسخ‌ها، گزینه‌ها، روابط و رسانه‌ها در job ذخیره می‌شود؛
هش جدا از متن هر فیلد هم در draft هست. کلید job=(lesson,locale,context_hash) و کلید
draft=(source_type,source_id,path,locale,context_hash) است. ترجمهٔ یک نسخه تکراری نمی‌شود.
روی اختلاف نسخه، save رد/وضعیت stale می‌شود. enqueue/refresh/export تغییر مبدأ را
با needs_review علامت می‌زنند؛ اجرای دوره‌ای refresh برای بازبینی پیوسته لازم است.
بدون trigger روی جداول اپلیکیشن، تغییرها بلافاصله در لحظهٔ ویرایش تشخیص داده نمی‌شوند.

سرویس SELECT روی پنج جدول مبدأ و نوشتن محدود روی localization دارد. اجازهٔ UPDATE
متن یا وضعیت تأیید draft ندارد؛ موارد human_approved را بازنویسی نمی‌کند. نقش بازبین
انسانی و انتشار عمداً در اختیار مدل نیست.

برای جلوگیری از ثبت نسخه قدیمی در رقابت با UPDATE یا INSERT، تابع ثابت
SECURITY DEFINER فقط پنج جدول مبدأ را در لحظه enqueue/save/refresh با SHARE قفل
می‌کند. ورودی SQL ندارد؛ search_path ثابت و EXECUTE عمومی لغو شده است. تا commit
ترجمه، ویرایش‌های مبدأ ممکن است مختصراً منتظر بمانند؛ خواندن‌ها ادامه دارند. این اثر باید
پیش از migration اصلی تأیید شود. در محیط پررفت‌وآمد، تعداد refresh را محدود کنید.

## اجرای قابل اعتماد و کیفیت

خواندن زمینه از MCP → نشست ترجمه → نشست تازهٔ بازبین → Pydantic و کنترل فنی → transaction ذخیره.
allowed_tools مدل فقط get_lesson_context/get_rules است؛ ابزار ثبت پیش‌نویس برای کنترل‌گر
با claim_token است. توکن claim در ورودی مدل فرستاده نمی‌شود.

claim با SKIP LOCKED و token و lease پیاده شده؛ heartbeat و سقف ۳ claim وجود دارد.
GETهای موقت حداکثر ۳ بار با backoff/jitter تکرار می‌شوند. create مبهم هرگز خودکار
دوباره ارسال نمی‌شود، تا هزینهٔ تکراری ایجاد نشود. شناسه نشست و خروجی در volume پایدار
با مجوز 0600 ذخیره می‌شوند. توقف، run را paused و نشست جاری را لغو می‌کند؛ resume پس از
پایان lease همان نشست را با Idempotency-Key ادامه می‌دهد. شکست قطعی نیازمند بررسی است.

معنا، طبیعی‌بودن و تشخیص زبان‌های نزدیک با regex تضمین نمی‌شود. کنترل‌های فنی:
تعداد/کلید فیلدها، locale، هش، placeholder/tag/blank، literal انگلیسی، Unicode و خط مقصد.
کنترل‌های بازبین: معنا، حذف/افزودن، واژه‌نامه، زبان و منطق آموزشی؛ همهٔ خروجی‌ها هنوز
needs_human_review هستند. کیفیت سورانی و خطاهای اختلاط زبان باید توسط گویشور بومی بررسی شوند.

## هزینه و مقایسه

سقف ۵ دلار در گفتگو برای پایلوت تأیید شده؛ هنوز هیچ درخواست پولی اجرا نشده است.
قیمت کوتاه استاندارد به‌ازای یک میلیون توکن در تاریخ بررسی:
Astra ورودی ۱۰ / خروجی ۵۰ دلار؛ Sol ورودی ۲ / خروجی ۱۰ دلار. این نرخ‌ها باید روز اجرا
دوباره با منبع رسمی کنترل شوند. o200k_base فقط tokenizer تقریبی است، نه ادعای tokenizer
رسمی این مدل‌ها. برآورد فعلی یک درس، دو زبان و دو مرحله شامل allowance reasoning:
Sol حدود ۰٫۵۸ دلار، Astra حدود ۲٫۹۰ دلار؛ جمع مقایسه حدود ۳٫۴۸ دلار، بدون تضمین هزینه.

مصرف گزارش‌شده best-effort است. کنترل‌گر برای هر مرحله سهمی ثابت از بودجه تخصیص می‌دهد،
نرخ بلند را محافظه‌کارانه اعمال می‌کند و با عبور از آستانه cancel می‌فرستد. این کنترل
سقف سخت دلاری از طرف OpenAI نیست و ممکن است تأخیر داشته باشد. CLI اجرای پولی را بدون
`--accept-soft-budget` نمی‌پذیرد. در صورت الزام سقف سخت، اجرای پولی تا احراز پشتیبانی
کنترل هزینهٔ سمت ارائه‌دهنده انجام نشود. از دو run جدا با سقف کامل ۵ دلار برای مقایسه
استفاده نکنید؛ مجموع مجوز پایلوت ۵ دلار است، نه ۵ دلار برای هر اجرا.

نمونه‌های ۹ نوع تمرین در tests/fixtures/real-exercise-samples.json هستند. ارزیابی انسانی
هر دو مدل باید با همین نمونه‌ها، زمینهٔ درس کامل و rubric گزارش انجام شود. هنوز مقایسهٔ
کیفیت، هزینهٔ واقعی یا زمان واقعی انجام نشده و برنده‌ای برای سورانی انتخاب نشده است.
