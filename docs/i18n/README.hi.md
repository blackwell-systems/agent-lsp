[English](../../README.md) · [简体中文](README.zh-CN.md) · [Русский](README.ru.md) · **हिन्दी** · [العربية](README.ar.md)

<p align="center">
  <img src="../../assets/banner.png" alt="agent-lsp" width="820">
</p>

<p align="center">
  <a href="#tools"><img src="https://img.shields.io/badge/CI--verified_tools-65%2F65-brightgreen.svg" alt="CI Coverage"></a>
  <a href="#multi-language-support"><img src="https://img.shields.io/badge/languages-31_CI--verified-brightgreen.svg" alt="Languages"></a>
  <a href="https://github.com/blackwell-systems/mcp-assert"><img src="https://raw.githubusercontent.com/blackwell-systems/mcp-assert/main/assets/badge-passing.svg?v=3" alt="mcp-assert: passing" height="20"></a>
  <a href="https://agentskills.io"><img src="../../assets/badge-agentskills.svg" alt="Agent Skills"></a>
  <a href="https://github.com/blackwell-systems/agent-lsp"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/blackwell-systems/agent-lsp/badges/assets/downloads-badge.json" alt="downloads"></a>
  <br>
  <a href="https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/"><img src="https://img.shields.io/badge/LSP-3.17-blue.svg" alt="LSP 3.17"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/punkpeye/awesome-mcp-servers"><img src="https://img.shields.io/badge/Awesome-MCP%20Servers-fc60a8" alt="Awesome MCP Servers"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

**AI एजेंट्स के लिए कोड इंटेलिजेंस इंफ्रास्ट्रक्चर।** 66 टूल, 31 CI-सत्यापित भाषाएँ, 24 एजेंट वर्कफ़्लो। एकल Go बाइनरी।

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh && agent-lsp init
```

## यह क्या है?

agent-lsp एक **MCP सर्वर** है जो मौजूदा LSP सर्वरों (gopls, rust-analyzer, jdtls, आदि) को एजेंट-नेटिव वर्कफ़्लो में व्यवस्थित करता है।

**यह कोई LSP सर्वर नहीं है** — यह एक ऑर्केस्ट्रेशन परत है जो लैंग्वेज सर्वरों का प्रबंधन करती है और MCP टूल के माध्यम से बैच ऑपरेशन, स्पेक्युलेटिव एडिटिंग, और बहु-चरणीय वर्कफ़्लो उपलब्ध कराती है।

**आर्किटेक्चर:**
- **लैंग्वेज सर्वर** (gopls, rust-analyzer, आदि) → कोड इंटेलिजेंस प्रदान करते हैं
- **agent-lsp** (MCP सर्वर) → वर्कफ़्लो व्यवस्थित करता है, वार्म रनटाइम बनाए रखता है
- **AI एजेंट** → MCP प्रोटोकॉल के माध्यम से उपभोग करते हैं

## agent-lsp क्यों?

**स्थायी वार्म रनटाइम**  
लैंग्वेज सर्वर एजेंट सत्रों के बीच इंडेक्स की हुई अवस्था में बने रहते हैं। पहला सत्र: वर्कस्पेस को इंडेक्स करता है (सामान्य परियोजनाओं के लिए ~10 सेकंड)। बाद के सत्र: तत्काल। हर अनुरोध पर कोई कोल्ड-स्टार्ट दंड नहीं।

**बैच ऑपरेशन**  
`blast_radius` → एक कॉल सभी एक्सपोर्ट्स + सभी कॉलर्स लौटाती है (टेस्ट बनाम नॉन-टेस्ट में विभाजित)। ऑर्केस्ट्रेशन के बिना: 20+ क्रमिक LSP कॉल।

**स्पेक्युलेटिव एडिटिंग**  
`simulate_edit` → मेमोरी में बदलावों का पूर्वावलोकन करें, डायग्नोस्टिक डेल्टा जाँचें, फिर लागू करें या छोड़ दें। डिस्क को छूने से पहले एडिट का परीक्षण करें।

**वर्कफ़्लो ऑर्केस्ट्रेशन**  
24 स्किल जो LSP ऑपरेशनों को पूर्ण पाइपलाइनों में श्रृंखलाबद्ध करती हैं:
- `/lsp-refactor` → प्रभाव विश्लेषण → पूर्वावलोकन → लागू करना → बिल्ड सत्यापन → परीक्षण चलाना
- `/lsp-safe-edit` → पूर्वावलोकन → डायग्नोस्टिक डिफ़ → सुरक्षित होने पर लागू करना
- `/lsp-verify` → LSP डायग्नोस्टिक्स → बिल्ड → टेस्ट सूट

**बहु-भाषा, एकल सत्र**  
एक agent-lsp प्रक्रिया `.go` को gopls, `.ts` को tsserver, `.py` को pyright पर रूट करती है। परियोजनाओं के बीच कोई पुनः-कॉन्फ़िगरेशन नहीं। सत्र फ़ाइलों और रिपॉज़िटरीज़ के बीच बना रहता है।

> [!TIP]
> **टोकन-अनुकूलित आउटपुट:** टूल प्रतिक्रियाएँ JSON के बजाय [GCF](https://gcformat.com) में एन्कोडेड होती हैं। टूल के आधार पर 30-84% कम टोकन (सत्र डीडुप के साथ 92.7% तक)। [हर फ्रंटियर मॉडल पर 100% LLM समझ](https://gcformat.com/guide/benchmarks.html), जटिल कोड ग्राफ़ पर 91.2%, जहाँ JSON औसतन 54.1% रहता है। प्रति-टूल मापी गई बचत के लिए [नीचे](#token-optimized-output-gcf) देखें।

**टुकड़े कैसे जुड़ते हैं:** [LSP](https://microsoft.github.io/language-server-protocol/) (Language Server Protocol) वह तरीका है जिससे एडिटर कोड इंटेलिजेंस प्राप्त करते हैं: कम्प्लीशन, डायग्नोस्टिक्स, गो-टू-डेफ़िनिशन। [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) वह मानक तरीका है जिससे Claude Code जैसे AI टूल बाहरी टूलों को खोजते और कॉल करते हैं। agent-lsp दोनों को जोड़ता है: लैंग्वेज सर्वर इंटेलिजेंस, AI एजेंट्स के लिए सुलभ।

## इसका उपयोग कब करें

- एजेंटिक कोड जनरेशन सिस्टम बनाते समय
- बड़े कोडबेस में रीफ़ैक्टर स्वचालित करते समय
- CI टूलिंग जिसे प्रोग्रामेटिक कोड इंटेलिजेंस चाहिए
- कोई भी वर्कफ़्लो जहाँ क्रमिक LSP कॉल बहुत धीमी या जटिल हों

### एजेंट क्या कहते हैं

हमने AI एजेंट्स से 10 कोडिंग कार्यों (कॉलर्स ढूँढना, सुरक्षित रूप से नाम बदलना, एडिट का पूर्वावलोकन, डेड कोड का पता लगाना) पर agent-lsp का मूल्यांकन करने और एक ईमानदार आकलन लिखने को कहा। चार अलग-अलग मॉडल, चार स्वतंत्र मूल्यांकन, एक ही निष्कर्ष:

> **Claude (Opus 4.6):** "रीफ़ैक्टरिंग, प्रभाव विश्लेषण, या सुरक्षित एडिटिंग से जुड़े किसी भी वर्कफ़्लो के लिए मैं agent-lsp की सिफ़ारिश करूँगा। सबसे उत्कृष्ट टूल हैं `blast_radius` (एक कॉल में प्रभाव क्षेत्र, टेस्ट/नॉन-टेस्ट विभाजन के साथ, जिसे दोहराने में 5-10 grep कमांड लगेंगे), `go_to_implementation` (टाइप-चेक्ड इंटरफ़ेस संतुष्टि, जो grep कर ही नहीं सकता), और सिमुलेशन सत्र वर्कफ़्लो (डिस्क को छुए बिना स्पेक्युलेटिव टाइप-चेकिंग, जिसका grep/read में कोई समकक्ष नहीं है)।"

> **Cursor (auto):** "भारी रीफ़ैक्टर और कोड नेविगेशन के लिए मैं agent-lsp की सिफ़ारिश करूँगा क्योंकि इसके रीनेम, रेफ़रेंस, इम्प्लीमेंटेशन, कॉल हायरार्की, और सिमुलेशन टूल बहुत सारे नाज़ुक grep/मैनुअल-एडिट काम को हटा देते हैं और बदलावों को सुरक्षित बनाते हैं।"

> **GPT-5.5 (Codex के माध्यम से):** "सिंबल-अवेयर काम के लिए मैं agent-lsp की सिफ़ारिश करूँगा: रेफ़रेंस, इम्प्लीमेंटेशन, रीनेम पूर्वावलोकन, डायग्नोस्टिक्स, और बड़ी-फ़ाइल संरचना grep/read लूप की तुलना में उल्लेखनीय रूप से तेज़ और कम त्रुटि-प्रवण हैं।"

> **Gemini 2.5 Pro (Gemini CLI के माध्यम से):** "मैं agent-lsp की सशक्त सिफ़ारिश करूँगा क्योंकि यह सिमेंटिक जागरूकता का ऐसा स्तर प्रदान करता है जिसकी बराबरी मानक टेक्स्ट-सर्च टूल कर ही नहीं सकते। डिस्क पर लिखे बिना उच्च-विश्वास वाले रीनेम करने, इंटरफ़ेस इम्प्लीमेंटेशन ढूँढने, और एडिट के डायग्नोस्टिक प्रभाव का पूर्वावलोकन करने की क्षमता रिग्रेशन आने के जोखिम को उल्लेखनीय रूप से घटाती है।"

### परखा गया, माना नहीं गया

हर दूसरा MCP-LSP कार्यान्वयन समर्थित भाषाओं को एक कॉन्फ़िग फ़ाइल में सूचीबद्ध करता है। उनमें से कोई भी यह सत्यापित करने के लिए CI में वास्तविक लैंग्वेज सर्वर नहीं चलाता कि वह काम करता है।

agent-lsp की CI हर push पर वास्तविक फ़िक्स्चर कोडबेस के विरुद्ध **32 वास्तविक लैंग्वेज सर्वर** चलाती है: Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, Clojure, Dart, Terraform, Nix, Prisma, SQL, MongoDB, MQL, और अधिक। जब हम कहते हैं "gopls के साथ काम करता है", तो यह एक सत्यापित, स्वचालित दावा है, कोई आशा नहीं।

### स्पेक्युलेटिव निष्पादन

डिस्क पर लिखने से पहले मेमोरी में बदलावों का सिमुलेशन करें। किसी अन्य MCP-LSP कार्यान्वयन में यह नहीं है।

`preview_edit` किसी भी एडिट के डायग्नोस्टिक प्रभाव का पूर्वावलोकन करता है। फ़ाइल को छूने से पहले आप ठीक-ठीक देख लेते हैं कि क्या टूटता है। `simulate_chain` निर्भर एडिट के एक अनुक्रम (एक फ़ंक्शन का नाम बदलना, सभी कॉलर्स को अपडेट करना, रिटर्न टाइप बदलना) का मूल्यांकन करता है और बताता है कि कौन-सा चरण पहले त्रुटि पैदा करता है।

8 स्पेक्युलेटिव निष्पादन टूल। पूर्ण वर्कफ़्लो के लिए [docs/guide/speculative-execution.md](../../docs/guide/speculative-execution.md) देखें।

### टोकन बचत

संरचित LSP प्रतिक्रियाएँ समान कार्यों पर grep/read की तुलना में **5-34 गुना कम टोकन** उपयोग करती हैं। HashiCorp Consul (3,19,000 लाइनें) पर, एक ब्लास्ट-रेडियस विश्लेषण grep के माध्यम से 17.7MB बनाम LSP के माध्यम से 841KB उपयोग करता है, जिससे 5,534 टूल कॉल घटकर 119 रह जाती हैं। बचत कोडबेस के आकार के साथ बढ़ती है। पाँच कोडबेस पर पूर्ण प्रयोग के लिए [docs/guide/token-savings.md](../../docs/guide/token-savings.md) देखें।

### टोकन-अनुकूलित आउटपुट (GCF)

टूल प्रतिक्रियाएँ JSON के बजाय [GCF (Graph Compact Format)](https://gcformat.com) में एन्कोडेड होती हैं। GCF फ़ील्ड-नाम की पुनरावृत्ति, आइडेंटिफ़ायर की पुनरावृत्ति, और प्रति-रिकॉर्ड संरचनात्मक ओवरहेड को समाप्त करता है।

| प्रोफ़ाइल | टूल | JSON की तुलना में बचत |
|---------|-------|----------------|
| Tabular | सभी 66 टूल | **30-51%** |
| Graph | blast_radius, find_callers, explore_symbol, find_references, type_hierarchy, cross_repo, detect_changes, list_symbols | **79-84%** |
| Graph + सत्र डीडुप | वही, [gcf-proxy](https://github.com/blackwell-systems/gcf-proxy) `--session` के माध्यम से | **92.7%** (5वीं कॉल) |

समूहीकृत/नेस्टेड प्रतिक्रियाएँ (सिंबल के तहत कॉलर्स, संबंधित जानकारी के साथ डायग्नोस्टिक्स) भी टेबुलराइज़ होती हैं, जो उस आकार पर JSON की तुलना में ~14% बचत देती हैं ([विवरण](../../docs/guide/gcf-integration.md#nested-container-responses-grouped-data))।

GCF डिफ़ॉल्ट रूप से सक्षम है। JSON पर लौटने के लिए:

```bash
export AGENT_LSP_OUTPUT_FORMAT=json
```

बेंचमार्क: `go run scripts/gcf-benchmark.go`. आर्किटेक्चर विवरण के लिए [docs/guide/gcf-integration.md](../../docs/guide/gcf-integration.md) देखें।

**GCF:** [gcformat.com](https://gcformat.com) · [Spec](https://github.com/blackwell-systems/gcf) · [Go](https://github.com/blackwell-systems/gcf-go) · [Python](https://github.com/blackwell-systems/gcf-python) · [TypeScript](https://github.com/blackwell-systems/gcf-typescript) · [Playground](https://gcformat.com/playground.html)

### ऑर्केस्ट्रेशन क्यों मायने रखता है

AI एजेंट गलत कोड बदलाव करते हैं क्योंकि वे पूरी तस्वीर नहीं देख पाते: इस फ़ंक्शन को कौन कॉल करता है, अगर मैं इसका नाम बदलूँ तो क्या टूटेगा, क्या बिल्ड अब भी पास होता है। लैंग्वेज सर्वरों के पास उत्तर हैं, लेकिन कच्चे LSP टूल को 20+ क्रमिक कॉल और जटिल ऑर्केस्ट्रेशन लॉजिक चाहिए।

agent-lsp इसे सही बहु-चरणीय ऑपरेशनों को एकल कॉल और स्किल में एन्कोड करके हल करता है। `blast_radius` वह काम एक कॉल में करता है जिसमें एजेंट को 20+ कॉल लगतीं। `/lsp-refactor` प्रभाव → पूर्वावलोकन → लागू करना → सत्यापन → परीक्षण को बिना प्रति-प्रॉम्प्ट ऑर्केस्ट्रेशन के श्रृंखलाबद्ध करता है।

### स्थायी डेमन मोड

Python और TypeScript परियोजनाओं को `find_references` के काम करने से पहले कई मिनट की बैकग्राउंड इंडेक्सिंग चाहिए। agent-lsp स्वचालित रूप से एक स्थायी डेमन ब्रोकर उत्पन्न करता है जो सत्रों के बीच बना रहता है, ताकि वर्कस्पेस इंडेक्स की हुई अवस्था में बना रहे। पहला सत्र: डेमन शुरू होता है और इंडेक्स करता है (FastAPI के लिए ~10 सेकंड)। बाद के सत्र: वार्म डेमन से तत्काल कनेक्शन। 30 मिनट की निष्क्रियता के बाद स्वतः बाहर निकलता है। Go, Rust, और अन्य तेज़-इंडेक्सिंग भाषाएँ इसे पूरी तरह बायपास करती हैं (शून्य ओवरहेड)।

### फ़ेज़ प्रवर्तन

स्किल एजेंट्स को ऑपरेशनों का सही क्रम बताती हैं। फ़ेज़ प्रवर्तन रनटाइम से उल्लंघनों को *अवरुद्ध* करवाता है, बजाय इसके कि एजेंट के निर्देशों का पालन करने पर भरोसा किया जाए।

जब एजेंट किसी स्किल को सक्रिय करता है, तो हर टूल कॉल की जाँच वर्तमान फ़ेज़ की अनुमतियों के विरुद्ध की जाती है। ब्लास्ट-रेडियस विश्लेषण के दौरान `apply_edit` कॉल करना चुपचाप आगे नहीं बढ़ता; यह विशिष्ट पुनर्प्राप्ति मार्गदर्शन के साथ एक त्रुटि लौटाता है ("पहले blast_radius फ़ेज़ पूरा करें, अनुमत टूल: [blast_radius, find_references]")। जैसे-जैसे एजेंट बाद के फ़ेज़ के टूल कॉल करता है, फ़ेज़ स्वचालित रूप से आगे बढ़ते हैं।

कोई अन्य MCP टूल प्रदाता रनटाइम पर वर्कफ़्लो क्रम को प्रवर्तित नहीं करता। [docs/guide/phase-enforcement.md](../../docs/guide/phase-enforcement.md) देखें।

### कॉनकरेंसी विश्लेषण

इंस्पेक्टर में 4 कॉनकरेंसी जाँचें शामिल हैं जो 4 कॉनकरेंसी परिवारों (goroutine, thread, async, actor) में 25 भाषाओं के आर-पार काम करती हैं:

- **अपुनर्प्राप्त कॉनकरेंट एंट्री**: पुनर्प्राप्ति के बिना goroutine/thread/task
- **अनियंत्रित साझा स्थिति**: sync.Map, ConcurrentHashMap पर नंगे टाइप ऐसर्शन
- **चैनल कभी बंद नहीं हुआ**: बनाए गए पर कभी बंद न हुए चैनल/क्यू (goroutine लीक)
- **सिंक के बिना साझा फ़ील्ड**: कॉनकरेंट संदर्भों से सिंक्रोनाइज़ेशन के बिना एक्सेस की गई फ़ील्ड

जब पैरेंट टाइप के पास म्यूटेक्स होता है, तो `blast_radius` सिंबलों को `sync_guarded: true` से एनोटेट करता है। `cross_concurrent: true` के साथ `find_callers` goroutine/thread सीमाओं के आर-पार कॉल श्रृंखलाओं का पता लगाता है। `/lsp-concurrency-audit` स्किल किसी भी टाइप के लिए फ़ील्ड-स्तरीय सुरक्षा रिपोर्ट तैयार करती है।

### ऑटो-डायग्नोस्टिक्स

सिंबल एडिट टूल (`replace_symbol_body`, `insert_after_symbol`, `insert_before_symbol`, `safe_delete_symbol`) स्वचालित रूप से `errors_after` और `warnings_after` गणनाएँ लौटाते हैं। एजेंट तुरंत जान लेते हैं कि किसी एडिट ने कुछ तोड़ा या नहीं, बिना अलग `get_diagnostics` कॉल के।

`safe_apply_edit` पूर्वावलोकन + लागू करना एक ही कॉल में जोड़ता है: स्पेक्युलेटिव रूप से पूर्वावलोकन करता है, और डिस्क पर केवल तभी लागू करता है जब `net_delta == 0` (कोई नई त्रुटि नहीं)। तीन के बजाय एक टूल कॉल।

### इनके साथ काम करता है

| AI टूल | ट्रांसपोर्ट | सेटअप |
|---------|-----------|-------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | stdio | `agent-lsp init` |
| [Cursor](https://cursor.com) | stdio | `agent-lsp init` |
| [Windsurf](https://windsurf.com) | stdio | `agent-lsp init` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | stdio | `agent-lsp init` |
| [Continue](https://continue.dev) | stdio | `agent-lsp init` |
| [Cline](https://github.com/cline/cline) | stdio | `agent-lsp init` |
| कोई भी MCP क्लाइंट | HTTP+SSE | `agent-lsp --http --port 8080` |

कॉपी-पेस्ट कॉन्फ़िग के लिए [docs/getting-started/mcp-clients.md](../../docs/getting-started/mcp-clients.md) देखें।

## स्किल

कच्चे टूल अनदेखे रह जाते हैं। स्किल उपयोग में आती हैं। हर स्किल सही टूल अनुक्रम को एन्कोड करती है ताकि वर्कफ़्लो वास्तव में बिना प्रति-प्रॉम्प्ट ऑर्केस्ट्रेशन निर्देशों के घटित हों। स्किल [AgentSkills](https://github.com/anthropics/agent-skills) स्लैश कमांड के रूप में और किसी भी MCP क्लाइंट के लिए `prompts/list` / `prompts/get` के माध्यम से MCP प्रॉम्प्ट के रूप में उपलब्ध हैं।

पूर्ण विवरण और उपयोग मार्गदर्शन के लिए [docs/guide/skills.md](../../docs/guide/skills.md) देखें।

**कुछ भी बदलने से पहले**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-impact` | किसी सिंबल या फ़ाइल को छूने से पहले ब्लास्ट-रेडियस विश्लेषण |
| `/lsp-implement` | किसी इंटरफ़ेस के सभी ठोस इम्प्लीमेंटेशन ढूँढना |
| `/lsp-dead-code` | सफ़ाई से पहले शून्य-रेफ़रेंस एक्सपोर्ट्स का पता लगाना |

**सुरक्षित रूप से एडिट करना**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-safe-edit` | डिस्क लेखन से पहले स्पेक्युलेटिव पूर्वावलोकन; पहले/बाद डायग्नोस्टिक डिफ़; त्रुटियों पर कोड एक्शन सामने लाता है |
| `/lsp-simulate` | फ़ाइल को छुए बिना मेमोरी में बदलावों का परीक्षण करना |
| `/lsp-edit-symbol` | किसी नामित सिंबल को उसकी फ़ाइल या स्थिति जाने बिना एडिट करना |
| `/lsp-edit-export` | एक्सपोर्ट किए गए सिंबलों का सुरक्षित एडिट, पहले सभी कॉलर्स ढूँढता है |
| `/lsp-rename` | `prepare_rename` सुरक्षा द्वार, सभी स्थलों का पूर्वावलोकन, पुष्टि, परमाणु रूप से लागू करना |

**शुरुआत करना**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-onboard` | पहले सत्र की परियोजना ऑनबोर्डिंग: भाषाएँ पहचानना, पैकेज मैप करना, एंट्री पॉइंट और हॉटस्पॉट ढूँढना, डायग्नोस्टिक्स जाँचना |

**अपरिचित कोड को समझना**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-explore` | "मुझे इस सिंबल के बारे में बताओ": एक ही पास में हॉवर + इम्प्लीमेंटेशन + कॉल हायरार्की + रेफ़रेंस |
| `/lsp-understand` | किसी सिंबल या फ़ाइल के लिए गहन कोड मैप: टाइप जानकारी, कॉल हायरार्की, रेफ़रेंस, स्रोत |
| `/lsp-docs` | तीन-स्तरीय दस्तावेज़: हॉवर → ऑफ़लाइन टूलचेन → स्रोत |
| `/lsp-cross-repo` | उपभोक्ता रिपॉज़िटरीज़ में किसी लाइब्रेरी सिंबल के सभी उपयोग ढूँढना |
| `/lsp-local-symbols` | फ़ाइल-दायरे की सिंबल सूची, उपयोग खोज, और टाइप जानकारी |

**एडिट करने के बाद**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-verify` | हर एडिट के बाद डायग्नोस्टिक्स + बिल्ड + टेस्ट |
| `/lsp-fix-all` | किसी फ़ाइल के सभी डायग्नोस्टिक्स के लिए क्विक-फ़िक्स कोड एक्शन लागू करना |
| `/lsp-test-correlation` | केवल वे टेस्ट ढूँढना और चलाना जो एडिट की गई फ़ाइल को कवर करते हैं |
| `/lsp-format-code` | लैंग्वेज सर्वर फ़ॉर्मैटर के माध्यम से किसी फ़ाइल या चयन को फ़ॉर्मैट करना |

**कोड जनरेट करना**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-generate` | सर्वर-साइड कोड जनरेशन ट्रिगर करना (इंटरफ़ेस स्टब, टेस्ट स्केलेटन, मॉक) |
| `/lsp-extract-function` | कोड एक्शन के माध्यम से किसी कोड ब्लॉक को नामित फ़ंक्शन में निकालना |

**पूर्ण वर्कफ़्लो**

| स्किल | उद्देश्य |
|-------|---------|
| `/lsp-refactor` | एंड-टू-एंड रीफ़ैक्टर: ब्लास्ट-रेडियस → पूर्वावलोकन → लागू करना → सत्यापन → टेस्ट |
| `/lsp-inspect` | पूर्ण कोड गुणवत्ता ऑडिट (12 जाँचें): डेड सिंबल, टेस्ट कवरेज, त्रुटि प्रबंधन, दस्तावेज़ ड्रिफ़्ट, कॉनकरेंसी सुरक्षा |
| `/lsp-concurrency-audit` | किसी टाइप के लिए फ़ील्ड-स्तरीय कॉनकरेंसी सुरक्षा ऑडिट: कॉनकरेंट एक्सेस का पता लगाता है, असिंक्रोनाइज़्ड फ़ील्ड चिह्नित करता है |

## Docker

**Stdio मोड** (MCP क्लाइंट कंटेनर को सीधे उत्पन्न करता है):

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio
```

**HTTP मोड** (स्थायी सेवा, दूरस्थ क्लाइंट HTTP+SSE पर कनेक्ट होते हैं):

```bash
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=your-secret-token \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 go:gopls
```

इमेज डिफ़ॉल्ट रूप से नॉन-रूट उपयोगकर्ता (uid 65532) के रूप में चलती हैं। `AGENT_LSP_TOKEN` को एनवायरनमेंट वेरिएबल के माध्यम से सेट करें, कभी भी कमांड लाइन पर `--token` से नहीं। इमेज Docker Hub (`blackwellsystems/agent-lsp`) पर भी मिरर की जाती हैं। पूर्ण टैग सूची, HTTP मोड सेटअप, और सुरक्षा सुदृढ़ीकरण विकल्पों के लिए [DOCKER.md](../../DOCKER.md) देखें।

## सेटअप

### चरण 1: agent-lsp इंस्टॉल करें

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

<details>
<summary>वैकल्पिक इंस्टॉल विधियाँ</summary>

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

**सभी प्लेटफ़ॉर्म**

```bash
# pip
pip install agent-lsp

# npm
npm install -g @blackwell-systems/agent-lsp

# Go install
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

</details>

### चरण 2: लैंग्वेज सर्वर इंस्टॉल करें

अपने स्टैक के लिए सर्वर इंस्टॉल करें। सामान्य वाले:

| भाषा | सर्वर | इंस्टॉल |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript@6` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

32 समर्थित भाषाओं की पूर्ण सूची [docs/reference/language-support.md](../../docs/reference/language-support.md) में।

### चरण 3: सेटअप सत्यापित करें

```bash
agent-lsp doctor
```

प्रत्येक कॉन्फ़िगर किए गए लैंग्वेज सर्वर की जाँच करता है और क्षमताओं की रिपोर्ट देता है। आगे बढ़ने से पहले किसी भी विफलता को ठीक करें। इंस्टॉल कमांड और सर्वर-विशिष्ट नोट्स के लिए [भाषा समर्थन](../../docs/reference/language-support.md) देखें।

### चरण 4: अपना AI टूल कॉन्फ़िगर करें

```bash
agent-lsp init
```

आपके PATH पर लैंग्वेज सर्वरों का पता लगाता है, पूछता है कि आप कौन-सा AI टूल उपयोग करते हैं, सही MCP कॉन्फ़िग लिखता है, और आपके AI प्रदाता के लिए स्किल जागरूकता नियम इंस्टॉल करता है (Claude Code के लिए CLAUDE.md, Cursor के लिए `.cursor/rules/`, Cline के लिए `.clinerules`, Windsurf के लिए `.windsurfrules`, Gemini CLI के लिए `GEMINI.md`)। CI या स्क्रिप्टेड उपयोग के लिए: `agent-lsp init --non-interactive`.

जनरेट किया गया कॉन्फ़िग ऐसा दिखता है:

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

प्रत्येक arg `language:server-binary` है (सर्वर args को कॉमा से अलग करें)।

### चरण 5: स्किल इंस्टॉल करें

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git /tmp/agent-lsp-skills
cd /tmp/agent-lsp-skills/skills && ./install.sh --copy
```

स्किल आपके AI टूल के कॉन्फ़िगरेशन में कॉपी की गई प्रॉम्प्ट फ़ाइलें हैं। `--copy` का अर्थ है कि क्लोन को बाद में सुरक्षित रूप से हटाया जा सकता है।

स्किल **MCP प्रॉम्प्ट** के रूप में भी उपलब्ध हैं: कोई भी MCP क्लाइंट उन्हें `prompts/list` के माध्यम से खोज सकता है और `prompts/get` के माध्यम से पूर्ण वर्कफ़्लो निर्देश प्राप्त कर सकता है, बिना किसी मैनुअल इंस्टॉलेशन के। `install.sh` पथ AgentSkills-संगत क्लाइंट (Claude Code स्लैश कमांड) के लिए है।

### चरण 6: टूल अनुमतियाँ दें (Claude Code)

Claude Code के लिए, अपनी अनुमति allow सूची में `mcp__lsp__*` जोड़ें ताकि सभी 65 टूल प्रति-टूल अनुमोदन प्रॉम्प्ट के बिना उपलब्ध हों:

```json
// ~/.claude/settings.json
{
  "permissions": {
    "allow": ["mcp__lsp__*"]
  }
}
```

इसके बिना, Claude Code हर टूल कॉल पर अनुमति के लिए प्रॉम्प्ट करेगा। अन्य MCP क्लाइंट अनुमतियाँ अलग तरह से संभालते हैं; अपने क्लाइंट के दस्तावेज़ जाँचें।

स्किल बहु-टूल वर्कफ़्लो हैं जो विश्वसनीय प्रक्रियाओं को एन्कोड करती हैं: एडिट से पहले ब्लास्ट-रेडियस जाँच, लेखन से पहले स्पेक्युलेटिव पूर्वावलोकन, बदलाव के बाद टेस्ट रन। पूर्ण सूची के लिए [docs/guide/skills.md](../../docs/guide/skills.md) देखें।

### चरण 7: काम शुरू करें

आपका AI एजेंट स्वचालित रूप से टूल कॉल करता है। पहली कॉल वर्कस्पेस को इनिशियलाइज़ करती है:

```
start_lsp(root_dir="/your/project")
```

यह वह है जो एजेंट करता है, कोई ऐसी चीज़ नहीं जो आप टाइप करते हैं। फिर 65 टूल में से किसी का भी उपयोग करें। सत्र वार्म बना रहता है; फ़ाइलें बदलते समय किसी पुनरारंभ की आवश्यकता नहीं।

## agent-lsp में अनूठा क्या है

| क्षमता | विवरण |
|------------|---------|
| टूल | **65** |
| भाषाएँ (CI-सत्यापित) | **32**, हर push पर एंड-टू-एंड इंटीग्रेशन टेस्ट |
| एजेंट वर्कफ़्लो (स्किल) | **24**, नामित बहु-चरणीय प्रक्रियाएँ, MCP `prompts/list` के माध्यम से खोजने योग्य |
| स्पेक्युलेटिव निष्पादन | **8 टूल**, डिस्क पर लिखने से पहले बदलावों का सिमुलेशन |
| फ़ेज़ प्रवर्तन | **4 स्किल**, रनटाइम क्रम-रहित टूल कॉल को पुनर्प्राप्ति मार्गदर्शन के साथ अवरुद्ध करता है |
| कनेक्शन मॉडल | **स्थायी**, फ़ाइलों और परियोजनाओं के आर-पार वार्म इंडेक्स |
| कॉल हायरार्की | **✓**, एकल टूल, दिशा पैरामीटर |
| टाइप हायरार्की | **✓**, CI-सत्यापित |
| क्रॉस-रेपो रेफ़रेंस | **✓**, बहु-रूट वर्कस्पेस |
| ऑटो-वॉच | **✓**, हमेशा-चालू, डिबाउंस्ड फ़ाइल वॉचिंग |
| HTTP+SSE ट्रांसपोर्ट | **✓**, bearer token प्रमाणीकरण, नॉन-रूट Docker |
| वितरण | **एकल Go बाइनरी**, 10 इंस्टॉल चैनल |

## उपयोग के मामले

- **बहु-परियोजना सत्र**: अपने AI को `~/code/` की ओर इंगित करें, बिना पुनः-कॉन्फ़िगर किए किसी भी परियोजना में काम करें
- **पॉलीग्लॉट विकास**: एक ही सत्र में Go बैकएंड + TypeScript फ्रंटएंड + Python स्क्रिप्ट
- **बड़े मोनोरेपो**: एक सर्वर सभी भाषाओं को संभालता है, फ़ाइल एक्सटेंशन के अनुसार रूट करता है
- **कोड माइग्रेशन**: पूर्ण क्रॉस-रेपो रेफ़रेंस ट्रैकिंग के साथ रिपॉज़िटरीज़ के आर-पार रीफ़ैक्टर
- **CI पाइपलाइन**: वास्तविक लैंग्वेज सर्वर व्यवहार के विरुद्ध सत्यापन
- **विशिष्ट भाषा स्टैक**: Gleam, Elixir, Prisma, Zig, Clojure, Nix, Dart, Scala, MongoDB, सभी CI-सत्यापित

## बहु-भाषा समर्थन

32 भाषाएँ, हर CI रन पर वास्तविक लैंग्वेज सर्वरों के विरुद्ध एंड-टू-एंड CI-सत्यापित। कोई अन्य MCP-LSP कार्यान्वयन CI में एक भी भाषा का परीक्षण नहीं करता।

Go, Python, TypeScript, Rust, Java, C, C++, C#, Ruby, PHP, Kotlin, Swift, Scala, Zig, Lua, Elixir, Gleam, Clojure, Dart, Terraform, Nix, Prisma, SQL, MongoDB, JavaScript, YAML, JSON, Dockerfile, CSS, HTML, MQL।

पूर्ण कवरेज मैट्रिक्स के लिए [docs/reference/language-support.md](../../docs/reference/language-support.md) देखें।

## टूल

65 टूल जो नेविगेशन, विश्लेषण, रीफ़ैक्टरिंग, सिंबल एडिटिंग, कंपोज़िट एक्सप्लोरेशन, सुरक्षित एडिटिंग, स्पेक्युलेटिव निष्पादन, और सत्र जीवनचक्र को कवर करते हैं। सभी CI-सत्यापित।

पैरामीटर और उदाहरणों के साथ पूर्ण संदर्भ के लिए [docs/reference/tools.md](../../docs/reference/tools.md) देखें।

## आगे पढ़ें

### दस्तावेज़

- [टूल संदर्भ](../../docs/reference/tools.md): पैरामीटर और उदाहरणों के साथ पूर्ण टूल संदर्भ
- [स्किल संदर्भ](../../docs/guide/skills.md): स्किल संदर्भ, वर्कफ़्लो, उपयोग के मामले, और संयोजन
- [भाषा समर्थन](../../docs/reference/language-support.md): भाषा कवरेज मैट्रिक्स
- [आर्किटेक्चर](../../docs/architecture/architecture.md): सिस्टम डिज़ाइन और आंतरिक कार्य
- [स्पेक्युलेटिव निष्पादन](../../docs/guide/speculative-execution.md): सिमुलेट-फिर-लागू करें वर्कफ़्लो
- [LSP अनुरूपता](../../docs/reference/lsp-conformance.md): LSP 3.17 स्पेक कवरेज
- [Docker](../../DOCKER.md): Docker टैग, compose, और वॉल्यूम कैशिंग

### योगदान

- [CI नोट्स](../../docs/architecture/ci-notes.md): CI विशेषताएँ और टेस्ट हार्नेस विवरण
- [वितरण](../../docs/architecture/distribution.md): इंस्टॉल चैनल और रिलीज़ पाइपलाइन

## विकास

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # unit tests
go test ./... -tags integration # integration tests (requires language servers)
```

## लाइब्रेरी उपयोग

`pkg/lsp`, `pkg/session`, और `pkg/types` पैकेज एक स्थिर Go API उजागर करते हैं जिससे MCP सर्वर चलाए बिना सीधे agent-lsp के LSP क्लाइंट का उपयोग किया जा सके।

```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

पूर्ण पैकेज API के लिए [docs/architecture/architecture.md](../../docs/architecture/architecture.md) देखें।

## लाइसेंस

MIT
