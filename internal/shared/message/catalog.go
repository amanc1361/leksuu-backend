// Package message is the single source of all user-facing text. Business and
// delivery code references stable codes; humans never see a code, and prose
// never appears inline anywhere else in the codebase.
package message

import "strings"

// Supported locales. The app's audience is Persian/Kurdish first.
const (
	LocaleFa      = "fa"
	LocaleCkb     = "ckb"
	LocaleEn      = "en"
	DefaultLocale = LocaleFa
)

var supported = map[string]struct{}{LocaleFa: {}, LocaleCkb: {}, LocaleEn: {}}

// catalog maps message code -> locale -> text.
var catalog = map[string]map[string]string{
	"auth.email_required": {
		LocaleEn:  "Email is required.",
		LocaleFa:  "ایمیل الزامی است.",
		LocaleCkb: "ئیمەیڵ پێویستە.",
	},
	"auth.email_invalid": {
		LocaleEn:  "Enter a valid email address.",
		LocaleFa:  "یک نشانی ایمیل معتبر وارد کنید.",
		LocaleCkb: "ناونیشانێکی ئیمەیڵی دروست بنووسە.",
	},
	"lesson.locked": {
		LocaleEn:  "This lesson is locked. Unlock it or subscribe to continue.",
		LocaleFa:  "این درس قفل است. برای ادامه آن را باز کنید یا اشتراک بگیرید.",
		LocaleCkb: "ئەم وانەیە داخراوە. بۆ بەردەوامبوون بیکەرەوە یان بەشداربە.",
	},
	"lesson.empty": {
		LocaleEn:  "This lesson has no content yet, so it cannot be completed.",
		LocaleFa:  "این درس هنوز محتوایی ندارد و نمی‌توان آن را تمام کرد.",
		LocaleCkb: "ئەم وانەیە هێشتا ناوەڕۆکی نییە، بۆیە ناتوانرێت تەواو بکرێت.",
	},
	"lesson.incomplete": {
		LocaleEn:  "Answer every exercise in the lesson before finishing it.",
		LocaleFa:  "پیش از پایان درس، به همهٔ تمرین‌های آن پاسخ دهید.",
		LocaleCkb: "پێش تەواوکردنی وانەکە وەڵامی هەموو ڕاهێنانەکانی بدەرەوە.",
	},
	"exercise.invalid_answer": {
		LocaleEn:  "The answer is incomplete or not valid for this exercise.",
		LocaleFa:  "پاسخ کامل نیست یا برای این تمرین معتبر نیست.",
		LocaleCkb: "وەڵامەکە تەواو نییە یان بۆ ئەم ڕاهێنانە دروست نییە.",
	},
	"exercise.misconfigured": {
		LocaleEn:  "This exercise cannot be checked right now.",
		LocaleFa:  "این تمرین در حال حاضر قابل بررسی نیست.",
		LocaleCkb: "ئەم ڕاهێنانە ئێستا ناتوانرێت پشکنین بکرێت.",
	},
	"exercise.no_assessment": {
		LocaleEn:  "Record yourself first so your pronunciation can be scored.",
		LocaleFa:  "ابتدا صدای خود را ضبط کنید تا تلفظ شما ارزیابی شود.",
		LocaleCkb: "سەرەتا دەنگی خۆت تۆمار بکە بۆ ئەوەی گۆکردنت هەڵبسەنگێنرێت.",
	},
	"billing.payments_unavailable": {
		LocaleEn:  "Online payment is not available right now. Please try again later.",
		LocaleFa:  "پرداخت آنلاین در حال حاضر در دسترس نیست. لطفاً بعداً دوباره تلاش کنید.",
		LocaleCkb: "پارەدانی ئۆنلاین ئێستا بەردەست نییە. تکایە دواتر دووبارە هەوڵ بدەرەوە.",
	},
	"game.daily_mix_incomplete": {
		LocaleEn:  "Finish today's reviews and practise first to claim the bonus.",
		LocaleFa:  "برای دریافت جایزه، ابتدا مرورها و تمرین امروز را انجام دهید.",
		LocaleCkb: "بۆ وەرگرتنی خەڵاتەکە، سەرەتا پێداچوونەوە و ڕاهێنانی ئەمڕۆ تەواو بکە.",
	},
	"auth.password_required": {
		LocaleEn:  "Password is required.",
		LocaleFa:  "رمز عبور الزامی است.",
		LocaleCkb: "وشەی نهێنی پێویستە.",
	},
	"auth.refresh_required": {
		LocaleEn:  "A refresh token is required.",
		LocaleFa:  "توکن تازه‌سازی الزامی است.",
		LocaleCkb: "تۆکنی نوێکردنەوە پێویستە.",
	},
	"auth.email_taken": {
		LocaleEn:  "This email is already registered.",
		LocaleFa:  "این ایمیل قبلاً ثبت شده است.",
		LocaleCkb: "ئەم ئیمەیڵە پێشتر تۆمارکراوە.",
	},
	"auth.invalid_credentials": {
		LocaleEn:  "Incorrect email or password.",
		LocaleFa:  "ایمیل یا رمز عبور نادرست است.",
		LocaleCkb: "ئیمەیڵ یان وشەی نهێنی هەڵەیە.",
	},
	"auth.token_invalid": {
		LocaleEn:  "The session token is invalid or expired.",
		LocaleFa:  "توکن نشست نامعتبر یا منقضی شده است.",
		LocaleCkb: "تۆکنی دانیشتن نادروست یان بەسەرچووە.",
	},
	"auth.token_reuse": {
		LocaleEn:  "Suspicious token reuse detected; the session was revoked.",
		LocaleFa:  "استفاده‌ی مشکوک از توکن شناسایی شد؛ نشست باطل شد.",
		LocaleCkb: "بەکارهێنانەوەی گومانلێکراوی تۆکن دۆزرایەوە؛ دانیشتن هەڵوەشایەوە.",
	},
	"auth.unauthenticated": {
		LocaleEn:  "Authentication is required.",
		LocaleFa:  "احراز هویت الزامی است.",
		LocaleCkb: "پشتڕاستکردنەوەی ناسنامە پێویستە.",
	},
	"auth.forbidden": {
		LocaleEn:  "You do not have permission for this action.",
		LocaleFa:  "شما اجازه‌ی این عمل را ندارید.",
		LocaleCkb: "تۆ مۆڵەتی ئەم کارەت نییە.",
	},
	"user.not_found": {
		LocaleEn:  "User not found.",
		LocaleFa:  "کاربر یافت نشد.",
		LocaleCkb: "بەکارهێنەر نەدۆزرایەوە.",
	},
	"common.internal": {
		LocaleEn:  "Something went wrong. Please try again.",
		LocaleFa:  "خطایی رخ داد. لطفاً دوباره تلاش کنید.",
		LocaleCkb: "هەڵەیەک ڕوویدا. تکایە دووبارە هەوڵبدەرەوە.",
	},
	"course.not_found": {
		LocaleEn:  "Course not found.",
		LocaleFa:  "دوره یافت نشد.",
		LocaleCkb: "خولەکە نەدۆزرایەوە.",
	},
	"lesson.not_found": {
		LocaleEn:  "Lesson not found.",
		LocaleFa:  "درس یافت نشد.",
		LocaleCkb: "وانەکە نەدۆزرایەوە.",
	},
	"lesson.source_conflict": {
		LocaleEn:  "This unit already has a lesson imported from the same source.",
		LocaleFa:  "این فصل از قبل درسی از همین منبع دارد.",
		LocaleCkb: "ئەم بەشە پێشتر وانەیەکی لە هەمان سەرچاوە هەیە.",
	},
	"course.item_not_found": {
		LocaleEn:  "Content item not found.",
		LocaleFa:  "آیتم محتوا پیدا نشد.",
		LocaleCkb: "بڕگەی ناوەڕۆک نەدۆزرایەوە.",
	},
	"exercise.not_found": {
		LocaleEn:  "Exercise not found.",
		LocaleFa:  "تمرین یافت نشد.",
		LocaleCkb: "ڕاهێنانەکە نەدۆزرایەوە.",
	},
	"srs.card_not_found": {
		LocaleEn:  "Review card not found.",
		LocaleFa:  "کارت مرور پیدا نشد.",
		LocaleCkb: "کارتی پێداچوونەوە نەدۆزرایەوە.",
	},
	"srs.vocab_not_found": {
		LocaleEn:  "Vocabulary item not found.",
		LocaleFa:  "واژه پیدا نشد.",
		LocaleCkb: "وشەکە نەدۆزرایەوە.",
	},
	"course.lesson_empty": {
		LocaleEn:  "This lesson has no content yet and can't be completed.",
		LocaleFa:  "این درس هنوز محتوایی ندارد و نمی‌توان آن را تمام کرد.",
		LocaleCkb: "ئەم وانەیە هێشتا ناوەڕۆکی نییە و ناکرێت تەواو بکرێت.",
	},
	"media.empty": {
		LocaleEn:  "No file provided.",
		LocaleFa:  "فایلی ارسال نشد.",
		LocaleCkb: "هیچ فایلێک نەنێردرا.",
	},
	"media.too_large": {
		LocaleEn:  "File is too large.",
		LocaleFa:  "فایل خیلی بزرگ است.",
		LocaleCkb: "فایلەکە زۆر گەورەیە.",
	},
	"media.bad_type": {
		LocaleEn:  "Only image, audio, or video files are allowed.",
		LocaleFa:  "فقط فایل عکس، صدا یا فیلم مجاز است.",
		LocaleCkb: "تەنها فایلی وێنە، دەنگ یان ڤیدیۆ ڕێگەپێدراوە.",
	},
	"media.fetch_failed": {
		LocaleEn:  "The file could not be downloaded right now. Please try again.",
		LocaleFa:  "دریافت فایل در حال حاضر ممکن نشد. دوباره تلاش کنید.",
		LocaleCkb: "ئێستا فایلەکە دانەبەزێنرا. تکایە دووبارە هەوڵ بدەرەوە.",
	},
	"language.invalid_code": {
		LocaleEn:  "Use a 2–3 letter ISO language code, e.g. it, es, nl.",
		LocaleFa:  "کدِ زبان باید ۲ یا ۳ حرفِ لاتینِ کوچک باشد، مثلاً it یا nl.",
		LocaleCkb: "کۆدی زمان دەبێت ٢ یان ٣ پیتی لاتینی بچووک بێت، بۆ نموونە it یان nl.",
	},
	"language.exists": {
		LocaleEn:  "This language is already in the list.",
		LocaleFa:  "این زبان از قبل در فهرست هست.",
		LocaleCkb: "ئەم زمانە پێشتر لە لیستەکەدایە.",
	},
	"language.not_found": {
		LocaleEn:  "Language not found.",
		LocaleFa:  "زبان پیدا نشد.",
		LocaleCkb: "زمان نەدۆزرایەوە.",
	},
	"user.invalid_target_lang": {
		LocaleEn:  "That language isn't available. Choose one of the languages offered.",
		LocaleFa:  "این زبان در دسترس نیست. یکی از زبان‌های موجود را انتخاب کن.",
		LocaleCkb: "ئەم زمانە بەردەست نییە. یەکێک لە زمانە بەردەستەکان هەڵبژێرە.",
	},
	"user.invalid_cefr": {
		LocaleEn:  "Invalid level.",
		LocaleFa:  "سطح نامعتبر است.",
		LocaleCkb: "ئاستی نادروست.",
	},
	"billing.class_subscription_required": {
		LocaleEn:  "A class subscription is required to access live classes.",
		LocaleFa:  "برای دسترسی به کلاس‌های زنده، اشتراکِ کلاس لازم است.",
		LocaleCkb: "بۆ دەستگەیشتن بە پۆلە زیندووەکان، بەشداری پۆل پێویستە.",
	},
	"billing.class_monthly": {
		LocaleEn:  "Monthly",
		LocaleFa:  "ماهانه",
		LocaleCkb: "مانگانە",
	},
	"billing.class_yearly": {
		LocaleEn:  "Yearly",
		LocaleFa:  "سالانه",
		LocaleCkb: "ساڵانە",
	},
	"billing.insufficient_gems": {
		LocaleEn:  "You don't have enough gems.",
		LocaleFa:  "جواهرِ کافی نداری.",
		LocaleCkb: "گەوهەری پێویستت نییە.",
	},
	"billing.needs_unlock": {
		LocaleEn:  "You've used today's free lessons. Unlock with gems or subscribe.",
		LocaleFa:  "درس‌های رایگانِ امروز تمام شد. با سکه باز کن یا اشتراک بگیر.",
		LocaleCkb: "وانە بەخۆڕاییەکانی ئەمڕۆ تەواو بوون. بە سکە بیکەرەوە یان بەشداری بکە.",
	},
	"billing.premium_required": {
		LocaleEn:  "This feature requires a premium subscription.",
		LocaleFa:  "این بخش نیازمندِ اشتراکِ ویژه است.",
		LocaleCkb: "ئەم بەشە پێویستی بە بەشداری تایبەتە.",
	},
	"billing.quota_exceeded": {
		LocaleEn:  "You've reached today's free limit. Upgrade to continue.",
		LocaleFa:  "به سقفِ رایگانِ امروز رسیدی. برای ادامه ارتقا بده.",
		LocaleCkb: "گەیشتیتە سنووری بەخۆڕاییِ ئەمڕۆ. بۆ بەردەوامبوون پلانەکەت بەرزبکەرەوە.",
	},
	"rtc.unauthorized": {
		LocaleEn:  "Invalid or expired session token.",
		LocaleFa:  "توکنِ نشست نامعتبر یا منقضی است.",
		LocaleCkb: "تۆکنی دانیشتن نادروست یان بەسەرچووە.",
	},
	"rtc.forbidden": {
		LocaleEn:  "You are not a participant of this class.",
		LocaleFa:  "تو عضوِ این کلاس نیستی.",
		LocaleCkb: "تۆ بەشدارێکی ئەم وانەیە نیت.",
	},
	"rtc.upgrade_failed": {
		LocaleEn:  "The live connection could not be established. A proxy or network may be blocking it.",
		LocaleFa:  "اتصالِ زنده برقرار نشد. ممکن است پراکسی یا شبکه آن را مسدود کرده باشد.",
		LocaleCkb: "پەیوەندی ڕاستەوخۆ دروست نەبوو. لەوانەیە پڕۆکسی یان تۆڕ ڕێی لێ گرتبێت.",
	},
	"assignment.not_found": {
		LocaleEn:  "Assignment not found.",
		LocaleFa:  "تمرین پیدا نشد.",
		LocaleCkb: "ئەرک نەدۆزرایەوە.",
	},
	"assignment.already_graded": {
		LocaleEn:  "This assignment was already graded.",
		LocaleFa:  "این تمرین قبلاً نمره داده شده است.",
		LocaleCkb: "ئەم ئەرکە پێشتر نمرەی پێدراوە.",
	},
	"assignment.not_submitted": {
		LocaleEn:  "The student hasn't submitted this yet.",
		LocaleFa:  "شاگرد هنوز این را ارسال نکرده است.",
		LocaleCkb: "خوێندکار هێشتا ئەمەی نەناردووە.",
	},
	"assignment.bad_score": {
		LocaleEn:  "Score must be between 0 and 100.",
		LocaleFa:  "نمره باید بینِ ۰ تا ۱۰۰ باشد.",
		LocaleCkb: "نمرە دەبێت لە نێوان ٠ بۆ ١٠٠ بێت.",
	},
	"assignment.bad_type": {
		LocaleEn:  "Unknown assignment type.",
		LocaleFa:  "نوعِ تمرین نامعتبر است.",
		LocaleCkb: "جۆری ئەرک نەناسراوە.",
	},
	"assignment.prompt_required": {
		LocaleEn:  "A task needs instructions.",
		LocaleFa:  "تمرین به دستورالعمل نیاز دارد.",
		LocaleCkb: "ئەرک پێویستی بە ڕێنمایی هەیە.",
	},
	"assignment.lesson_required": {
		LocaleEn:  "Choose a lesson for this assignment.",
		LocaleFa:  "برای این تمرین یک درس انتخاب کن.",
		LocaleCkb: "بۆ ئەم ئەرکە وانەیەک هەڵبژێرە.",
	},
	"tutor.not_your_student": {
		LocaleEn:  "This learner is not your student.",
		LocaleFa:  "این فراگیر شاگردِ تو نیست.",
		LocaleCkb: "ئەم فێرخوازە خوێندکاری تۆ نییە.",
	},
	"classroom.not_found": {
		LocaleEn:  "Class not found.",
		LocaleFa:  "کلاس پیدا نشد.",
		LocaleCkb: "پۆل نەدۆزرایەوە.",
	},
	"classroom.full": {
		LocaleEn:  "This class is full.",
		LocaleFa:  "ظرفیتِ این کلاس تکمیل است.",
		LocaleCkb: "ئەم پۆلە پڕە.",
	},
	"classroom.already_enrolled": {
		LocaleEn:  "You're already enrolled in this class.",
		LocaleFa:  "تو قبلاً در این کلاس ثبت‌نام کرده‌ای.",
		LocaleCkb: "تۆ پێشتر لەم پۆلە تۆمارکراویت.",
	},
	"classroom.finished": {
		LocaleEn:  "This class has already finished.",
		LocaleFa:  "این کلاس تمام شده است.",
		LocaleCkb: "ئەم پۆلە کۆتایی هاتووە.",
	},
	"classroom.start_past": {
		LocaleEn:  "Pick a future start time.",
		LocaleFa:  "زمانِ شروع باید در آینده باشد.",
		LocaleCkb: "کاتی دەستپێکی داهاتوو هەڵبژێرە.",
	},
	"tutor.already_applied": {
		LocaleEn:  "You've already applied to teach.",
		LocaleFa:  "قبلاً برای تدریس درخواست داده‌ای.",
		LocaleCkb: "پێشتر داواکاریت بۆ وانەوتنەوە کردووە.",
	},
	"tutor.already_tutor": {
		LocaleEn:  "You're already a tutor.",
		LocaleFa:  "تو همین‌حالا مدرس هستی.",
		LocaleCkb: "تۆ ئێستا مامۆستایت.",
	},
	"tutor.user_not_found": {
		LocaleEn:  "No user found with that email.",
		LocaleFa:  "کاربری با این ایمیل پیدا نشد.",
		LocaleCkb: "بەکارهێنەرێک بەو ئیمەیڵە نەدۆزرایەوە.",
	},
	"auth.invalid_status": {
		LocaleEn:  "Invalid account status.",
		LocaleFa:  "وضعیتِ حساب نامعتبر است.",
		LocaleCkb: "دۆخی هەژمار نادروستە.",
	},
	"auth.cannot_suspend_self": {
		LocaleEn:  "You can't suspend your own account.",
		LocaleFa:  "نمی‌توانی حسابِ خودت را معلق کنی.",
		LocaleCkb: "ناتوانیت هەژماری خۆت ڕاگریت.",
	},
	"auth.role_not_manageable": {
		LocaleEn:  "This role can't be changed here.",
		LocaleFa:  "این نقش از اینجا قابلِ‌تغییر نیست.",
		LocaleCkb: "ئەم ڕۆڵە لێرەوە ناگۆڕدرێت.",
	},
	"auth.user_not_found": {
		LocaleEn:  "User not found.",
		LocaleFa:  "کاربر یافت نشد.",
		LocaleCkb: "بەکارهێنەر نەدۆزرایەوە.",
	},
	"auth.cannot_demote_self": {
		LocaleEn:  "You can't remove your own admin role.",
		LocaleFa:  "نمی‌توانی نقشِ ادمینِ خودت را حذف کنی.",
		LocaleCkb: "ناتوانیت ڕۆڵی ئەدمینی خۆت بسڕیتەوە.",
	},
	"tutor.not_a_tutor": {
		LocaleEn:  "Your account is not linked to a tutor profile.",
		LocaleFa:  "حسابِ تو به پروفایلِ مدرس متصل نیست.",
		LocaleCkb: "هەژمارەکەت بە پرۆفایلی مامۆستا نەبەستراوەتەوە.",
	},
	"tutor.not_found": {
		LocaleEn:  "Tutor not found.",
		LocaleFa:  "مدرس پیدا نشد.",
		LocaleCkb: "مامۆستا نەدۆزرایەوە.",
	},
	"tutor.slot_not_found": {
		LocaleEn:  "That time slot no longer exists.",
		LocaleFa:  "این بازه‌ی زمانی دیگر وجود ندارد.",
		LocaleCkb: "ئەم کاتە چیتر بوونی نییە.",
	},
	"tutor.slot_taken": {
		LocaleEn:  "That slot was just booked. Please pick another.",
		LocaleFa:  "این زمان همین حالا رزرو شد. لطفاً زمانِ دیگری انتخاب کن.",
		LocaleCkb: "ئەم کاتە هەر ئێستا گیرا. تکایە کاتێکی تر هەڵبژێرە.",
	},
	"tutor.slot_past": {
		LocaleEn:  "That time is in the past.",
		LocaleFa:  "این زمان گذشته است.",
		LocaleCkb: "ئەم کاتە ڕابردووە.",
	},
	"tutor.booking_not_found": {
		LocaleEn:  "Booking not found.",
		LocaleFa:  "رزرو پیدا نشد.",
		LocaleCkb: "حیجزکردن نەدۆزرایەوە.",
	},
	"tutor.too_late_to_cancel": {
		LocaleEn:  "This session can no longer be cancelled.",
		LocaleFa:  "این جلسه دیگر قابلِ لغو نیست.",
		LocaleCkb: "ئەم دانیشتنە چیتر ناتوانرێت هەڵبوەشێتەوە.",
	},
	"tutor.bad_rating": {
		LocaleEn:  "Rating must be between 1 and 5.",
		LocaleFa:  "امتیاز باید بینِ ۱ تا ۵ باشد.",
		LocaleCkb: "خاڵ دەبێت لە نێوان ١ بۆ ٥ بێت.",
	},
	"tutor.session_not_finished": {
		LocaleEn:  "You can review a session after it ends.",
		LocaleFa:  "بعد از پایانِ جلسه می‌توانی نظر بدهی.",
		LocaleCkb: "دوای کۆتایی دانیشتن دەتوانیت پێداچوونەوە بنووسیت.",
	},
	"tutor.already_reviewed": {
		LocaleEn:  "You already reviewed this session.",
		LocaleFa:  "قبلاً برای این جلسه نظر داده‌ای.",
		LocaleCkb: "پێشتر پێداچوونەوەت بۆ ئەم دانیشتنە نووسیوە.",
	},
	"tutor.booking_cancelled": {
		LocaleEn:  "This booking was cancelled.",
		LocaleFa:  "این رزرو لغو شده است.",
		LocaleCkb: "ئەم حیجزکردنە هەڵوەشێنرایەوە.",
	},
	"game.reward_already_claimed": {
		LocaleEn:  "You've already claimed today's reward. Come back tomorrow!",
		LocaleFa:  "جایزه‌ی امروز را گرفته‌ای. فردا برگرد!",
		LocaleCkb: "خەڵاتی ئەمڕۆت وەرگرتووە. سبەینێ بگەڕێوە!",
	},
	"billing.payment_failed": {
		LocaleEn:  "Payment could not be completed.",
		LocaleFa:  "پرداخت کامل نشد.",
		LocaleCkb: "پارەدان تەواو نەبوو.",
	},
	"pronunciation.bad_output": {
		LocaleEn:  "The pronunciation service returned an unexpected format.",
		LocaleFa:  "سرویسِ تلفظ خروجیِ نامعتبر داد.",
		LocaleCkb: "خزمەتگوزاری بێژەکردن دەرئەنجامێکی نادروستی گەڕاندەوە.",
	},
	"ai.bad_output": {
		LocaleEn:  "The AI returned an unexpected format. Please try again.",
		LocaleFa:  "خروجیِ هوش مصنوعی در قالبِ نامعتبر بود. دوباره تلاش کن.",
		LocaleCkb: "دەرئەنجامی AI لە فۆرماتێکی نادروستدا بوو. دووبارە هەوڵبدە.",
	},
	"dictionary.provider_not_found": {
		LocaleEn:  "Dictionary provider not found.",
		LocaleFa:  "ارائه‌دهنده‌ی فرهنگ لغت پیدا نشد.",
		LocaleCkb: "دابینکەری فەرهەنگ نەدۆزرایەوە.",
	},
	"dictionary.no_result": {
		LocaleEn:  "No meaning found for that word.",
		LocaleFa:  "برای این واژه معنایی پیدا نشد.",
		LocaleCkb: "هیچ واتایەک بۆ ئەو وشەیە نەدۆزرایەوە.",
	},
	"common.bad_request": {
		LocaleEn:  "The request could not be understood.",
		LocaleFa:  "درخواست قابل پردازش نبود.",
		LocaleCkb: "داواکارییەکە تێنەگەیشترا.",
	},
}

// Localize returns the text for a code in the best supported locale, falling
// back to the default locale, then to the code itself (never panics).
func Localize(locale, code string) string {
	if byLocale, ok := catalog[code]; ok {
		if txt, ok := byLocale[normalize(locale)]; ok {
			return txt
		}
		if txt, ok := byLocale[DefaultLocale]; ok {
			return txt
		}
	}
	return code
}

func normalize(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if _, ok := supported[locale]; ok {
		return locale
	}
	return DefaultLocale
}

// Negotiate picks a supported locale from an Accept-Language header value.
func Negotiate(acceptLanguage string) string {
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag := strings.TrimSpace(part)
		if i := strings.IndexByte(tag, ';'); i >= 0 {
			tag = tag[:i]
		}
		tag = strings.ToLower(tag)
		// match primary subtag (e.g. "fa-IR" -> "fa")
		if i := strings.IndexByte(tag, '-'); i >= 0 {
			tag = tag[:i]
		}
		if _, ok := supported[tag]; ok {
			return tag
		}
	}
	return DefaultLocale
}
