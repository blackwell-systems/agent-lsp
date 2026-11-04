[English](../../README.md) · [简体中文](README.zh-CN.md) · [Русский](README.ru.md) · [हिन्दी](README.hi.md) · **العربية**

<p align="center">
  <img src="../../assets/banner.png" alt="agent-lsp" width="820">
</p>

<p align="center">
  <a href="#tools"><img src="https://img.shields.io/badge/CI--verified_tools-65%2F65-brightgreen.svg" alt="CI Coverage"></a>
  <a href="#multi-language-support"><img src="https://img.shields.io/badge/languages-30_CI--verified-brightgreen.svg" alt="Languages"></a>
  <a href="https://github.com/blackwell-systems/mcp-assert"><img src="https://raw.githubusercontent.com/blackwell-systems/mcp-assert/main/assets/badge-passing.svg?v=3" alt="mcp-assert: passing" height="20"></a>
  <a href="https://agentskills.io"><img src="../../assets/badge-agentskills.svg" alt="Agent Skills"></a>
  <a href="https://github.com/blackwell-systems/agent-lsp"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/blackwell-systems/agent-lsp/badges/assets/downloads-badge.json" alt="downloads"></a>
  <br>
  <a href="https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/"><img src="https://img.shields.io/badge/LSP-3.17-blue.svg" alt="LSP 3.17"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/punkpeye/awesome-mcp-servers"><img src="https://img.shields.io/badge/Awesome-MCP%20Servers-fc60a8" alt="Awesome MCP Servers"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

**بنية تحتية لذكاء الشيفرة موجَّهة لوكلاء الذكاء الاصطناعي.** 65 أداة، و32 لغة مُتحقَّقًا منها عبر CI، و24 سير عمل للوكلاء. ملف Go تنفيذي واحد.

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh && agent-lsp init
```

## ما هذا؟

agent-lsp هو **خادم MCP** يُنسِّق خوادم LSP القائمة (gopls وrust-analyzer وjdtls، وغيرها) ضمن سيرورات عمل موجَّهة للوكلاء.

**ليس خادم LSP** — بل هو طبقة تنسيق تُدير خوادم اللغة وتُتيح العمليات الدُّفعية والتحرير التخميني وسيرورات العمل متعددة الخطوات عبر أدوات MCP.

**البنية:**
- **خوادم اللغة** (gopls وrust-analyzer، وغيرها) → تُوفِّر ذكاء الشيفرة
- **agent-lsp** (خادم MCP) → يُنسِّق سيرورات العمل ويُبقي بيئة تشغيل ساخنة
- **وكلاء الذكاء الاصطناعي** → يستهلكونها عبر بروتوكول MCP

## لماذا agent-lsp؟

**بيئة تشغيل ساخنة ودائمة**  
تبقى خوادم اللغة مُفهرسة عبر جلسات الوكيل. الجلسة الأولى: تُفهرس مساحة العمل (نحو 10 ثوانٍ للمشاريع النموذجية). الجلسات اللاحقة: فورية. لا عقوبة بدء بارد عند كل طلب.

**العمليات الدُّفعية**  
`blast_radius` → استدعاء واحد يُعيد كل الرموز المُصدَّرة + كل المُستدعِين (مُقسَّمين إلى اختباريين وغير اختباريين). بدون التنسيق: أكثر من 20 استدعاء LSP متتاليًا.

**التحرير التخميني**  
`simulate_edit` → عايِن التغييرات في الذاكرة، وافحص فرق التشخيص، ثم طبِّق أو تجاهل. اختبِر التحريرات قبل المساس بالقرص.

**تنسيق سيرورات العمل**  
24 مهارة تربط عمليات LSP في خطوط أنابيب كاملة:
- `/lsp-refactor` → تحليل الأثر → معاينة → تطبيق → التحقق من البناء → تشغيل الاختبارات
- `/lsp-safe-edit` → معاينة → فرق التشخيص → تطبيق إذا كان آمنًا
- `/lsp-verify` → تشخيصات LSP → بناء → مجموعة الاختبارات

**متعددة اللغات، جلسة واحدة**  
تُوجِّه عملية agent-lsp واحدة الملفات `.go` إلى gopls، و`.ts` إلى tsserver، و`.py` إلى pyright. لا إعادة تهيئة بين المشاريع. تبقى الجلسة قائمة عبر الملفات والمستودعات.

> [!TIP]
> **مُخرَجات مُحسَّنة للرموز (tokens):** تُرمَّز استجابات الأدوات بصيغة [GCF](https://gcformat.com) بدلًا من JSON. رموز أقل بنسبة 30-84% بحسب الأداة (حتى 92.7% مع إزالة التكرار على مستوى الجلسة). [فهم LLM بنسبة 100% على كل نموذج متقدم](https://gcformat.com/guide/benchmarks.html)، و91.2% على رسوم الشيفرة المعقدة حيث يبلغ متوسط JSON فيها 54.1%. راجع [أدناه](#token-optimized-output-gcf) للاطلاع على التوفير المقيس لكل أداة.

**كيف تتكامل القطع معًا:** [LSP](https://microsoft.github.io/language-server-protocol/) (Language Server Protocol) هو الطريقة التي تحصل بها المُحرِّرات على ذكاء الشيفرة: الإكمالات والتشخيصات والانتقال إلى التعريف. [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) هو الطريقة المِعيارية التي تكتشف بها أدوات الذكاء الاصطناعي مثل Claude Code الأدوات الخارجية وتستدعيها. يجسِّر agent-lsp بينهما: ذكاء خادم اللغة، مُتاحًا لوكلاء الذكاء الاصطناعي.

## استخدمها عندما

- تبني أنظمة توليد شيفرة وكيلية
- تُؤتمِت عمليات إعادة الهيكلة عبر قواعد شيفرة كبيرة
- أدوات CI تحتاج إلى ذكاء شيفرة برمجي
- أي سير عمل تكون فيه استدعاءات LSP المتتالية بطيئة أو معقدة أكثر من اللازم

### ماذا يقول الوكلاء

طلبنا من وكلاء الذكاء الاصطناعي تقييم agent-lsp عبر 10 مهام برمجية (إيجاد المُستدعِين، إعادة التسمية بأمان، معاينة التحريرات، كشف الشيفرة الميتة) وكتابة تقييم صادق. أربعة نماذج مختلفة، وأربعة تقييمات مستقلة، والخلاصة نفسها:

> **Claude (Opus 4.6):** "سأوصي بـ agent-lsp لأي سير عمل يتضمن إعادة الهيكلة أو تحليل الأثر أو التحرير الآمن. الأدوات البارزة هي `blast_radius` (نطاق الأثر في استدعاء واحد، مع تقسيم اختباري/غير اختباري يستلزم تكراره 5-10 أوامر grep)، و`go_to_implementation` (تحقُّق من استيفاء الواجهات مفحوصًا بالأنواع، وهو ما لا يقدر عليه grep ببساطة)، وسير عمل جلسة المحاكاة (فحص أنواع تخميني دون المساس بالقرص، وليس له أي مكافئ في grep/read على الإطلاق)."

> **Cursor (auto):** "سأوصي بـ agent-lsp لعمليات إعادة الهيكلة الثقيلة والتنقل في الشيفرة لأن أدوات إعادة التسمية والمراجع والتنفيذات وتسلسل الاستدعاءات والمحاكاة تُزيل قدرًا كبيرًا من عمل grep/التحرير اليدوي الهش وتجعل التغييرات أكثر أمانًا."

> **GPT-5.5 (عبر Codex):** "سأوصي بـ agent-lsp للعمل المدرِك للرموز: المراجع والتنفيذات ومعاينات إعادة التسمية والتشخيصات وبنية الملفات الكبيرة أسرع بشكل ملموس وأقل عُرضة للأخطاء من حلقات grep/read."

> **Gemini 2.5 Pro (عبر Gemini CLI):** "أوصي بشدة بـ agent-lsp لأنه يوفر مستوى من الإدراك الدلالي لا تقدر أدوات البحث النصي القياسية على مضاهاته ببساطة. إن القدرة على إجراء عمليات إعادة تسمية عالية الثقة، وإيجاد تنفيذات الواجهات، ومعاينة الأثر التشخيصي للتحريرات دون الكتابة إلى القرص، تُقلِّل بشكل ملموس من خطر إدخال الانحدارات."

### مُختبَرة، لا مُفترَضة

كل تنفيذ آخر لـ MCP-LSP يُدرِج اللغات المدعومة في ملف تهيئة. لا أحد منها يُشغِّل خادم اللغة الفعلي في CI للتحقق من أنه يعمل.

يُشغِّل CI الخاص بـ agent-lsp **32 خادم لغة حقيقيًا** على قواعد شيفرة تثبيتية حقيقية عند كل push: Go وPython وTypeScript وRust وJava وC وC++ وC# وRuby وPHP وKotlin وSwift وScala وZig وLua وElixir وGleam وClojure وDart وTerraform وNix وPrisma وSQL وMongoDB وMQL، وغيرها. حين نقول "يعمل مع gopls"، فهذا ادعاء مُتحقَّق منه وآلي، لا أمنية.

### التنفيذ التخميني

حاكِ التغييرات في الذاكرة قبل الكتابة إلى القرص. لا يمتلك هذا أيُّ تنفيذ آخر لـ MCP-LSP.

يُعايِن `preview_edit` الأثر التشخيصي لأي تحرير. ترى بالضبط ما الذي سيتعطل قبل المساس بالملف. ويُقيِّم `simulate_chain` سلسلة من التحريرات المترابطة (إعادة تسمية دالة، وتحديث كل المُستدعِين، وتغيير نوع القيمة المُعادة) ويُبلِّغ عن الخطوة التي تُدخِل خطأً أولًا.

8 أدوات تنفيذ تخميني. راجع [docs/guide/speculative-execution.md](../../docs/guide/speculative-execution.md) للاطلاع على سير العمل الكامل.

### توفير الرموز

تستخدم استجابات LSP المُهيكَلة **رموزًا أقل بمقدار 5-34 ضعفًا** من grep/read في المهام نفسها. على HashiCorp Consul (319 ألف سطر)، يستخدم تحليل نطاق الأثر 17.7 ميغابايت عبر grep مقابل 841 كيلوبايت عبر LSP، مُقلِّصًا 5,534 استدعاء أداة إلى 119. يتوسَّع التوفير مع حجم قاعدة الشيفرة. راجع [docs/guide/token-savings.md](../../docs/guide/token-savings.md) للاطلاع على التجربة الكاملة عبر خمس قواعد شيفرة.

### مُخرَجات مُحسَّنة للرموز (GCF)

تُرمَّز استجابات الأدوات بصيغة [GCF (Graph Compact Format)](https://gcformat.com) بدلًا من JSON. تُزيل GCF تكرار أسماء الحقول، وتكرار المُعرِّفات، والحِمل البنيوي لكل سجل.

| الملف التعريفي | الأدوات | التوفير مقابل JSON |
|---------|-------|----------------|
| Tabular | كل الأدوات الـ66 | **30-51%** |
| Graph | blast_radius، find_callers، explore_symbol، find_references، type_hierarchy، cross_repo، detect_changes، list_symbols | **79-84%** |
| Graph + إزالة التكرار على مستوى الجلسة | نفسها، عبر [gcf-proxy](https://github.com/blackwell-systems/gcf-proxy) `--session` | **92.7%** (الاستدعاء الخامس) |

تُحوَّل الاستجابات المُجمَّعة/المتداخلة (المُستدعُون تحت رمز، والتشخيصات مع معلومات متصلة) إلى جداول أيضًا، بما يوفِّر نحو 14% مقابل JSON على ذلك الشكل ([التفاصيل](../../docs/guide/gcf-integration.md#nested-container-responses-grouped-data)).

GCF مُفعَّلة افتراضيًا. للعودة إلى JSON:

```bash
export AGENT_LSP_OUTPUT_FORMAT=json
```

قياس الأداء: `go run scripts/gcf-benchmark.go`. راجع [docs/guide/gcf-integration.md](../../docs/guide/gcf-integration.md) للاطلاع على تفاصيل البنية.

**GCF:** [gcformat.com](https://gcformat.com) · [Spec](https://github.com/blackwell-systems/gcf) · [Go](https://github.com/blackwell-systems/gcf-go) · [Python](https://github.com/blackwell-systems/gcf-python) · [TypeScript](https://github.com/blackwell-systems/gcf-typescript) · [Playground](https://gcformat.com/playground.html)

### لماذا يهم التنسيق

يُجري وكلاء الذكاء الاصطناعي تغييرات شيفرة غير صحيحة لأنهم لا يرون الصورة الكاملة: من يستدعي هذه الدالة، وما الذي يتعطل إن أعدتُ تسميتها، وهل ما زال البناء ينجح. تمتلك خوادم اللغة الإجابات، لكن أدوات LSP الخام تتطلب أكثر من 20 استدعاءً متتاليًا ومنطق تنسيق معقدًا.

يحل agent-lsp هذا بترميز العمليات متعددة الخطوات الصحيحة في استدعاءات ومهارات مفردة. يُنجِز `blast_radius` في استدعاء واحد ما كان سيستلزم من الوكيل أكثر من 20 استدعاءً. ويربط `/lsp-refactor` الأثر → المعاينة → التطبيق → التحقق → الاختبار دون تنسيق لكل مُوجَّه.

### وضع الخفيّ (daemon) الدائم

تحتاج مشاريع Python وTypeScript إلى دقائق من الفهرسة في الخلفية قبل أن يعمل `find_references`. يُنشئ agent-lsp تلقائيًا وسيط خفيّ دائمًا يبقى بين الجلسات، بحيث تبقى مساحة العمل مُفهرسة. الجلسة الأولى: يبدأ الخفيّ ويُفهرس (نحو 10 ثوانٍ لـ FastAPI). الجلسات اللاحقة: اتصال فوري بالخفيّ الساخن. يخرج تلقائيًا بعد 30 دقيقة من الخمول. وتتجاوز Go وRust واللغات السريعة الفهرسة الأخرى هذا كليًا (بلا حِمل).

### فرض الأطوار

تُخبر المهارات الوكلاء بالترتيب الصحيح للعمليات. يجعل فرض الأطوار بيئة التشغيل *تحظر* المخالفات بدلًا من الوثوق بأن الوكيل سيتبع التعليمات.

عندما يُفعِّل الوكيل مهارة، يُفحَص كل استدعاء أداة مقابل صلاحيات الطور الحالي. لا يمضي استدعاء `apply_edit` أثناء تحليل نطاق الأثر بصمت؛ بل يُعيد خطأً مع إرشاد استرداد محدد ("أكمِل طور blast_radius أولًا، الأدوات المسموح بها: [blast_radius, find_references]"). تتقدم الأطوار تلقائيًا مع استدعاء الوكيل أدوات من أطوار لاحقة.

لا يفرض أيُّ مُزوِّد أدوات MCP آخر ترتيب سير العمل في زمن التشغيل. راجع [docs/guide/phase-enforcement.md](../../docs/guide/phase-enforcement.md).

### تحليل التزامن

يتضمن المُفتِّش 4 فحوص تزامن تعمل عبر 25 لغة في 4 عائلات تزامن (goroutine وthread وasync وactor):

- **مدخل متزامن غير مُستردّ**: goroutines/threads/tasks بلا استرداد
- **حالة مشتركة غير مفحوصة**: تأكيدات نوع عارية على sync.Map وConcurrentHashMap
- **قناة لم تُغلَق قط**: قنوات/طوابير أُنشئت لكن لم تُغلَق قط (تسريبات goroutine)
- **حقل مشترك بلا مزامنة**: حقول يُوصَل إليها من سياقات متزامنة دون مزامنة

يُعلِّم `blast_radius` الرموز بـ `sync_guarded: true` عندما يمتلك النوع الأب مِزلاجًا (mutex). ويتتبَّع `find_callers` مع `cross_concurrent: true` سلاسل الاستدعاء عبر حدود goroutine/thread. وتُنتج مهارة `/lsp-concurrency-audit` تقرير أمان على مستوى الحقول لأي نوع.

### التشخيصات التلقائية

تُعيد أدوات تحرير الرموز (`replace_symbol_body` وinsert_after_symbol` وinsert_before_symbol` وsafe_delete_symbol`) تلقائيًا عدَّي `errors_after` و`warnings_after`. يعرف الوكلاء فورًا ما إذا كان تحريرٌ ما قد عطَّل شيئًا دون استدعاء `get_diagnostics` منفصل.

يجمع `safe_apply_edit` المعاينة والتطبيق في استدعاء واحد: يُعاين تخمينيًا، ويُطبِّق على القرص فقط إذا كان `net_delta == 0` (لا أخطاء جديدة). استدعاء أداة واحد بدلًا من ثلاثة.

### يعمل مع

| أداة الذكاء الاصطناعي | النقل | الإعداد |
|---------|-----------|-------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | stdio | `agent-lsp init` |
| [Cursor](https://cursor.com) | stdio | `agent-lsp init` |
| [Windsurf](https://windsurf.com) | stdio | `agent-lsp init` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | stdio | `agent-lsp init` |
| [Continue](https://continue.dev) | stdio | `agent-lsp init` |
| [Cline](https://github.com/cline/cline) | stdio | `agent-lsp init` |
| أي عميل MCP | HTTP+SSE | `agent-lsp --http --port 8080` |

راجع [docs/getting-started/mcp-clients.md](../../docs/getting-started/mcp-clients.md) للاطلاع على تهيئات جاهزة للنسخ واللصق.

## المهارات

تُتجاهَل الأدوات الخام. أما المهارات فتُستخدَم. تُرمِّز كل مهارة تسلسل الأدوات الصحيح بحيث تحدث سيرورات العمل فعلًا دون تعليمات تنسيق لكل مُوجَّه. تتوفر المهارات كأوامر شرطة مائلة عبر [AgentSkills](https://github.com/anthropics/agent-skills) وكمُوجَّهات MCP عبر `prompts/list` / `prompts/get` لأي عميل MCP.

راجع [docs/guide/skills.md](../../docs/guide/skills.md) للاطلاع على الأوصاف الكاملة وإرشادات الاستخدام.

**قبل أن تُغيِّر أي شيء**

| المهارة | الغرض |
|-------|---------|
| `/lsp-impact` | تحليل نطاق الأثر قبل المساس برمز أو ملف |
| `/lsp-implement` | إيجاد كل التنفيذات الملموسة لواجهة |
| `/lsp-dead-code` | كشف الرموز المُصدَّرة عديمة المراجع قبل التنظيف |

**التحرير بأمان**

| المهارة | الغرض |
|-------|---------|
| `/lsp-safe-edit` | معاينة تخمينية قبل الكتابة على القرص؛ فرق تشخيص قبلي/بعدي؛ يُظهِر إجراءات الشيفرة عند الأخطاء |
| `/lsp-simulate` | اختبار التغييرات في الذاكرة دون المساس بالملف |
| `/lsp-edit-symbol` | تحرير رمز مُسمًّى دون معرفة ملفه أو موضعه |
| `/lsp-edit-export` | تحرير آمن للرموز المُصدَّرة، يجد كل المُستدعِين أولًا |
| `/lsp-rename` | بوابة أمان `prepare_rename`، ومعاينة كل المواضع، والتأكيد، والتطبيق ذريًا |

**البدء**

| المهارة | الغرض |
|-------|---------|
| `/lsp-onboard` | تهيئة المشروع في الجلسة الأولى: كشف اللغات، ورسم خريطة الحزم، وإيجاد نقاط الدخول والبؤر الساخنة، وفحص التشخيصات |

**فهم الشيفرة غير المألوفة**

| المهارة | الغرض |
|-------|---------|
| `/lsp-explore` | "أخبِرني عن هذا الرمز": تمرير + تنفيذات + تسلسل استدعاءات + مراجع في تمريرة واحدة |
| `/lsp-understand` | خريطة شيفرة مُعمَّقة لرمز أو ملف: معلومات النوع، وتسلسل الاستدعاءات، والمراجع، والمصدر |
| `/lsp-docs` | توثيق ثلاثي المستويات: تمرير → سلسلة أدوات دون اتصال → مصدر |
| `/lsp-cross-repo` | إيجاد كل استخدامات رمز مكتبة عبر مستودعات المُستهلِكين |
| `/lsp-local-symbols` | قائمة رموز بنطاق الملف، وبحث في الاستخدامات، ومعلومات النوع |

**بعد التحرير**

| المهارة | الغرض |
|-------|---------|
| `/lsp-verify` | تشخيصات + بناء + اختبارات بعد كل تحرير |
| `/lsp-fix-all` | تطبيق إجراءات إصلاح سريع لكل التشخيصات في ملف |
| `/lsp-test-correlation` | إيجاد وتشغيل الاختبارات التي تُغطي ملفًا مُحرَّرًا فقط |
| `/lsp-format-code` | تنسيق ملف أو تحديد عبر مُنسِّق خادم اللغة |

**توليد الشيفرة**

| المهارة | الغرض |
|-------|---------|
| `/lsp-generate` | تشغيل توليد شيفرة على جانب الخادم (كعوب واجهات، هياكل اختبارات، أدوات محاكاة) |
| `/lsp-extract-function` | استخراج كتلة شيفرة إلى دالة مُسمّاة عبر إجراءات الشيفرة |

**سير العمل الكامل**

| المهارة | الغرض |
|-------|---------|
| `/lsp-refactor` | إعادة هيكلة شاملة: نطاق الأثر → معاينة → تطبيق → تحقق → اختبار |
| `/lsp-inspect` | تدقيق كامل لجودة الشيفرة (12 فحصًا): الرموز الميتة، وتغطية الاختبار، ومعالجة الأخطاء، وانحراف التوثيق، وأمان التزامن |
| `/lsp-concurrency-audit` | تدقيق أمان تزامن على مستوى الحقول لنوع: يتتبَّع الوصول المتزامن، ويُعلِّم الحقول غير المُزامَنة |

## Docker

**وضع Stdio** (يُنشئ عميل MCP الحاوية مباشرة):

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio
```

**وضع HTTP** (خدمة دائمة، يتصل العملاء البُعداء عبر HTTP+SSE):

```bash
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=your-secret-token \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

تعمل الصور افتراضيًا بمستخدم غير جذر (uid 65532). عيِّن `AGENT_LSP_TOKEN` عبر متغير بيئة، ولا تستخدم `--token` في سطر الأوامر أبدًا. تُنسَخ الصور أيضًا إلى Docker Hub (`blackwellsystems/agent-lsp`). راجع [DOCKER.md](../../DOCKER.md) للاطلاع على قائمة الوسوم الكاملة، وإعداد وضع HTTP، وخيارات تعزيز الأمان.

## الإعداد

### الخطوة 1: ثبِّت agent-lsp

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

<details>
<summary>طرق تثبيت بديلة</summary>

**macOS / Linux**

```bash
brew install blackwell-systems/tap/agent-lsp
```

**Windows**

```powershell
# PowerShell (no admin required)
iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex

# Scoop
scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp
scoop install blackwell-systems/agent-lsp

# Winget
winget install BlackwellSystems.agent-lsp
```

**كل المنصات**

```bash
# pip
pip install agent-lsp

# npm
npm install -g @blackwell-systems/agent-lsp

# Go install
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

</details>

### الخطوة 2: ثبِّت خوادم اللغة

ثبِّت الخوادم الخاصة بمجموعتك التقنية. الشائعة منها:

| اللغة | الخادم | التثبيت |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript@6` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

القائمة الكاملة للغات المدعومة الـ32 في [docs/reference/language-support.md](../../docs/reference/language-support.md).

### الخطوة 3: تحقَّق من الإعداد

```bash
agent-lsp doctor
```

يفحص كل خادم لغة مُهيَّأ ويُبلِّغ عن قدراته. أصلِح أي إخفاقات قبل المتابعة. راجع [دعم اللغات](../../docs/reference/language-support.md) للاطلاع على أوامر التثبيت والملاحظات الخاصة بكل خادم.

### الخطوة 4: هيِّئ أداة الذكاء الاصطناعي لديك

```bash
agent-lsp init
```

يكتشف خوادم اللغة على PATH لديك، ويسأل عن أداة الذكاء الاصطناعي التي تستخدمها، ويكتب تهيئة MCP الصحيحة، ويُثبِّت قواعد الوعي بالمهارات لمُزوِّد الذكاء الاصطناعي لديك (CLAUDE.md لـ Claude Code، و`.cursor/rules/` لـ Cursor، و`.clinerules` لـ Cline، و`.windsurfrules` لـ Windsurf، وGEMINI.md لـ Gemini CLI). للاستخدام في CI أو ضمن النصوص البرمجية: `agent-lsp init --non-interactive`.

تبدو التهيئة المُولَّدة هكذا:

```json
{
  "mcpServers": {
    "lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": [
        "go:gopls",
        "typescript:typescript-language-server,--stdio",
        "python:pyright-langserver,--stdio"
      ]
    }
  }
}
```

كل وسيط هو `language:server-binary` (افصِل وسائط الخادم بفواصل).

### الخطوة 5: ثبِّت المهارات

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git /tmp/agent-lsp-skills
cd /tmp/agent-lsp-skills/skills && ./install.sh --copy
```

المهارات ملفات مُوجَّهات تُنسَخ إلى تهيئة أداة الذكاء الاصطناعي لديك. `--copy` يعني أنه يمكن حذف النسخة المستنسخة بأمان بعد ذلك.

تتوفر المهارات أيضًا كـ **مُوجَّهات MCP**: يمكن لأي عميل MCP اكتشافها عبر `prompts/list` وجلب تعليمات سير العمل الكاملة عبر `prompts/get`، دون أي تثبيت يدوي. مسار `install.sh` مُخصَّص للعملاء المتوافقين مع AgentSkills (أوامر الشرطة المائلة في Claude Code).

### الخطوة 6: اسمح بصلاحيات الأدوات (Claude Code)

بالنسبة إلى Claude Code، أضِف `mcp__lsp__*` إلى قائمة السماح بالصلاحيات لديك بحيث تكون كل الأدوات الـ65 متاحة دون مُوجَّهات موافقة لكل أداة:

```json
// ~/.claude/settings.json
{
  "permissions": {
    "allow": ["mcp__lsp__*"]
  }
}
```

بدون هذا، سيطلب Claude Code الإذن عند كل استدعاء أداة. تتعامل عملاء MCP الآخرون مع الصلاحيات بشكل مختلف؛ راجع وثائق عميلك.

المهارات سيرورات عمل متعددة الأدوات تُرمِّز إجراءات موثوقة: فحص نطاق الأثر قبل التحرير، ومعاينة تخمينية قبل الكتابة، وتشغيل الاختبارات بعد التغيير. راجع [docs/guide/skills.md](../../docs/guide/skills.md) للاطلاع على القائمة الكاملة.

### الخطوة 7: ابدأ العمل

يستدعي وكيل الذكاء الاصطناعي لديك الأدوات تلقائيًا. يُهيِّئ الاستدعاء الأول مساحة العمل:

```
start_lsp(root_dir="/your/project")
```

هذا ما يفعله الوكيل، لا شيء تكتبه أنت. ثم استخدم أيًّا من الأدوات الـ65. تبقى الجلسة ساخنة؛ لا حاجة إلى إعادة تشغيل عند تبديل الملفات.

## ما المميَّز في agent-lsp

| القدرة | التفاصيل |
|------------|---------|
| الأدوات | **65** |
| اللغات (مُتحقَّق منها عبر CI) | **32**، اختبارات تكامل شاملة عند كل push |
| سيرورات عمل الوكلاء (المهارات) | **24**، إجراءات مُسمّاة متعددة الخطوات، قابلة للاكتشاف عبر MCP `prompts/list` |
| التنفيذ التخميني | **8 أدوات**، محاكاة التغييرات قبل الكتابة إلى القرص |
| فرض الأطوار | **4 مهارات**، تحظر بيئة التشغيل استدعاءات الأدوات خارج الترتيب مع إرشاد استرداد |
| نموذج الاتصال | **دائم**، فهرس ساخن عبر الملفات والمشاريع |
| تسلسل الاستدعاءات | **✓**، أداة واحدة، مُعامِل اتجاه |
| تسلسل الأنواع | **✓**، مُتحقَّق منه عبر CI |
| المراجع عبر المستودعات | **✓**، مساحة عمل متعددة الجذور |
| المراقبة التلقائية | **✓**، دائمة التشغيل، مراقبة ملفات مع كبح الارتداد |
| نقل HTTP+SSE | **✓**، مصادقة bearer token، Docker غير جذري |
| التوزيع | **ملف Go تنفيذي واحد**، 10 قنوات تثبيت |

## حالات الاستخدام

- **جلسات متعددة المشاريع**: وجِّه ذكاءك الاصطناعي إلى `~/code/`، واعمل عبر أي مشروع دون إعادة تهيئة
- **التطوير متعدد اللغات**: خلفية Go + واجهة TypeScript + نصوص Python في جلسة واحدة
- **المستودعات الأحادية الكبيرة**: خادم واحد يتولى كل اللغات، ويُوجِّه بحسب امتداد الملف
- **ترحيل الشيفرة**: إعادة هيكلة عبر المستودعات مع تتبُّع كامل للمراجع عبر المستودعات
- **خطوط أنابيب CI**: التحقق مقابل السلوك الفعلي لخادم اللغة
- **مجموعات اللغات المتخصصة**: Gleam وElixir وPrisma وZig وClojure وNix وDart وScala وMongoDB، جميعها مُتحقَّق منها عبر CI

## دعم اللغات المتعددة

32 لغة، مُتحقَّق منها بشكل شامل عبر CI مقابل خوادم لغة حقيقية عند كل تشغيل CI. لا يختبر أيُّ تنفيذ آخر لـ MCP-LSP ولو لغة واحدة في CI.

Go وPython وTypeScript وRust وJava وC وC++ وC# وRuby وPHP وKotlin وSwift وScala وZig وLua وElixir وGleam وClojure وDart وTerraform وNix وPrisma وSQL وMongoDB وJavaScript وYAML وJSON وDockerfile وCSS وHTML وMQL.

راجع [docs/reference/language-support.md](../../docs/reference/language-support.md) للاطلاع على مصفوفة التغطية الكاملة.

## الأدوات

65 أداة تُغطي التنقل والتحليل وإعادة الهيكلة وتحرير الرموز والاستكشاف المُركَّب والتحرير الآمن والتنفيذ التخميني ودورة حياة الجلسة. جميعها مُتحقَّق منها عبر CI.

راجع [docs/reference/tools.md](../../docs/reference/tools.md) للاطلاع على المرجع الكامل مع المُعامِلات والأمثلة.

## قراءات إضافية

### التوثيق

- [مرجع الأدوات](../../docs/reference/tools.md): مرجع أدوات كامل مع المُعامِلات والأمثلة
- [مرجع المهارات](../../docs/guide/skills.md): مرجع المهارات وسيرورات العمل وحالات الاستخدام والتركيب
- [دعم اللغات](../../docs/reference/language-support.md): مصفوفة تغطية اللغات
- [البنية](../../docs/architecture/architecture.md): تصميم النظام وآلياته الداخلية
- [التنفيذ التخميني](../../docs/guide/speculative-execution.md): سيرورات عمل "حاكِ ثم طبِّق"
- [توافق LSP](../../docs/reference/lsp-conformance.md): تغطية مواصفة LSP 3.17
- [Docker](../../DOCKER.md): وسوم Docker وcompose وتخزين الأحجام مؤقتًا

### المساهمة

- [ملاحظات CI](../../docs/architecture/ci-notes.md): خصائص CI وتفاصيل عُدَّة الاختبار
- [التوزيع](../../docs/architecture/distribution.md): قنوات التثبيت وخط الإصدار

## التطوير

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # unit tests
go test ./... -tags integration # integration tests (requires language servers)
```

## الاستخدام كمكتبة

تُتيح الحزم `pkg/lsp` و`pkg/session` و`pkg/types` واجهة برمجة Go مستقرة لاستخدام عميل LSP الخاص بـ agent-lsp مباشرةً دون تشغيل خادم MCP.

```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

راجع [docs/architecture/architecture.md](../../docs/architecture/architecture.md) للاطلاع على واجهة برمجة الحزم الكاملة.

## الرخصة

MIT
