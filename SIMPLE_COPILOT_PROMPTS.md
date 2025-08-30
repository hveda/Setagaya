# Simple Copilot Agent Prompts

## 🎯 **Option 1: Full Implementation**

```
@github-copilot implement the production UI according to PRODUCTION_UI_IMPLEMENTATION_PROMPT.md

Transform app.html from Vue.js to Alpine.js + Tailwind CSS with RBAC integration. Preserve all Go template variables and maintain backward compatibility.
```

## 🔧 **Option 2: Phase-by-Phase**

```
@github-copilot implement Phase 1 from PRODUCTION_UI_IMPLEMENTATION_PROMPT.md

Replace Vue.js with Alpine.js in app.html while preserving all Go template variables ({{ .Context }}, {{ .IsAdmin }}, etc.).
```

## 🎨 **Option 3: Specific Component**

```
@github-copilot create Alpine.js stores following PRODUCTION_UI_IMPLEMENTATION_PROMPT.md

Create ui/static/js/app.js with appStore(), projectManager(), and adminPanel() functions that integrate with /api/rbac/* endpoints.
```

## 🔐 **Option 4: RBAC Integration**

```
@github-copilot add RBAC controls to app.html using PRODUCTION_UI_IMPLEMENTATION_PROMPT.md

Add permission-based visibility with x-show="hasPermission('resource:action')" for navigation and UI elements.
```

## 🛠 **Option 5: Route Updates**

```
@github-copilot update ui/handler.go routes per PRODUCTION_UI_IMPLEMENTATION_PROMPT.md

Add production routes (/admin, /projects, /collections) and remove demo .html extensions while preserving homeHandler template variables.
```

---

**Pick the option that matches your current need, or combine multiple prompts for broader implementation.**
