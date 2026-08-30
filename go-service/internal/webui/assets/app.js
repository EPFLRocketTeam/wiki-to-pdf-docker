function el(id) {
  return document.getElementById(id);
}

function queryParam(name) {
  return new URLSearchParams(window.location.search).get(name);
}

async function postJSON(path, payload) {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });
  return response;
}

async function getJSON(path) {
  const response = await fetch(path);
  const data = await response.json();
  return { response, data };
}

function appendOutput(message) {
  const output = el("output");
  if (!output) return;
  const stamp = new Date().toISOString();
  output.textContent = `[${stamp}] ${message}\n` + output.textContent;
}

async function initMonacoEditors() {
  const markdownHost = el("markdownEditor");
  const latexHost = el("latexEditor");

  if (!markdownHost || !latexHost) {
    return {
      getMarkdown: () => "",
      setMarkdown: () => {},
      getLatex: () => "",
      setLatex: () => {}
    };
  }

  if (!window.require) {
    appendOutput("Monaco loader missing; falling back to plain mode.");
    return {
      markdownValue: "",
      latexValue: "",
      getMarkdown() { return this.markdownValue; },
      setMarkdown(v) { this.markdownValue = v || ""; },
      getLatex() { return this.latexValue; },
      setLatex(v) { this.latexValue = v || ""; }
    };
  }

  return new Promise((resolve) => {
    window.require.config({ paths: { vs: "https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.47.0/min/vs" } });
    window.require(["vs/editor/editor.main"], () => {
      const markdownEditor = monaco.editor.create(markdownHost, {
        value: "",
        language: "markdown",
        theme: "vs",
        automaticLayout: true,
        wordWrap: "on",
        minimap: { enabled: false }
      });

      const latexEditor = monaco.editor.create(latexHost, {
        value: "",
        language: "latex",
        theme: "vs",
        automaticLayout: true,
        wordWrap: "on",
        minimap: { enabled: false }
      });

      resolve({
        getMarkdown: () => markdownEditor.getValue(),
        setMarkdown: (v) => markdownEditor.setValue(v || ""),
        getLatex: () => latexEditor.getValue(),
        setLatex: (v) => latexEditor.setValue(v || ""),
        layout: () => {
          markdownEditor.layout();
          latexEditor.layout();
        }
      });
    });
  });
}

function currentDateISO() {
  return new Date().toISOString().slice(0, 10);
}

function parseURLs(raw) {
  return raw
    .split(/\r?\n/)
    .map((v) => v.trim())
    .filter((v) => v.length > 0);
}

function buildOverleafURL(sessionID, title) {
  const baseUrl = window.location.origin;
  const zipUri = `${baseUrl}/serve-zip-project/${sessionID}`;
  const encodedZip = encodeURIComponent(zipUri);
  const encodedMain = encodeURIComponent("main.tex");
  const encodedTitle = encodeURIComponent(title || "Wiki Document");
  return `https://www.overleaf.com/docs?snip_uri[]=${encodedZip}&main_document=${encodedMain}&title=${encodedTitle}`;
}

async function mountIndex() {
  const convertBtn = el("convertBtn");
  const compileBtn = el("compileBtn");
  const overleafBtn = el("overleafBtn");
  const fetchBtn = el("fetchBtn");
  const tokenBtn = el("tokenBtn");
  const pageSelector = el("pageSelector");

  if (!convertBtn || !compileBtn || !fetchBtn || !tokenBtn || !pageSelector) {
    return;
  }

  const editors = await initMonacoEditors();
  const pages = [];
  let lastConvertSessionID = "";
  let lastPDFBlobURL = "";

  function getMeta() {
    return {
      author: el("author").value || "",
      date: el("date").value || currentDateISO(),
      title: el("title").value || "",
      documentId: el("documentId").value || "",
      template: el("template").value || "space-race"
    };
  }

  function applyPageData(page) {
    editors.setMarkdown(page.content || "");
    el("author").value = page.authorName || "";
    el("title").value = page.title || "";
    if (!el("date").value) {
      el("date").value = currentDateISO();
    }
  }

  pageSelector.addEventListener("change", () => {
    const idx = Number(pageSelector.value);
    if (!Number.isNaN(idx) && pages[idx]) {
      applyPageData(pages[idx]);
    }
  });

  tokenBtn.addEventListener("click", async () => {
    tokenBtn.disabled = true;
    try {
      const payload = {
        endpointUrl: el("graphqlUrl").value.trim(),
        username: el("username").value.trim(),
        password: el("password").value
      };
      const response = await postJSON("/get-access-token", payload);
      const data = await response.json();
      if (!response.ok) {
        appendOutput("Token fetch failed: " + JSON.stringify(data));
        return;
      }
      el("token").value = data.token || "";
      appendOutput("Token fetched successfully.");
    } catch (error) {
      appendOutput("Token fetch error: " + error);
    } finally {
      tokenBtn.disabled = false;
    }
  });

  fetchBtn.addEventListener("click", async () => {
    fetchBtn.disabled = true;
    try {
      const urls = parseURLs(el("pageUrls").value);
      if (urls.length === 0) {
        appendOutput("No page URLs provided.");
        return;
      }

      const payload = {
        urls,
        graphql_url: el("graphqlUrl").value.trim(),
        token: el("token").value.trim()
      };
      const response = await postJSON("/fetch", payload);
      const data = await response.json();
      if (!response.ok) {
        appendOutput("Fetch failed: " + JSON.stringify(data));
        return;
      }

      pages.length = 0;
      pageSelector.innerHTML = "";
      data.forEach((item, idx) => {
        pages.push(item);
        const option = document.createElement("option");
        option.value = String(idx);
        option.textContent = item.path || item.title || `page-${idx + 1}`;
        pageSelector.appendChild(option);
      });

      if (pages.length > 0) {
        pageSelector.value = "0";
        applyPageData(pages[0]);
      }
      appendOutput(`Fetched ${pages.length} page(s).`);
    } catch (error) {
      appendOutput("Fetch error: " + error);
    } finally {
      fetchBtn.disabled = false;
      if (editors.layout) editors.layout();
    }
  });

  convertBtn.addEventListener("click", async () => {
    convertBtn.disabled = true;
    el("convertMeta").textContent = "Converting...";
    try {
      const meta = getMeta();
      const response = await postJSON("/convert", {
        markdown: editors.getMarkdown(),
        author: meta.author,
        date: meta.date,
        title: meta.title,
        documentId: meta.documentId,
        footerText: "",
        template: meta.template,
        lineNumbersEnabled: false
      });
      const data = await response.json();
      if (!response.ok) {
        appendOutput("Convert failed: " + JSON.stringify(data));
        el("convertMeta").textContent = "Conversion failed";
        return;
      }

      editors.setLatex(data.latex || "");
      lastConvertSessionID = data.session_id || "";
      el("convertMeta").textContent = "Converted successfully";
      appendOutput("Convert success. Session ID: " + (lastConvertSessionID || "none"));
    } catch (error) {
      appendOutput("Convert error: " + error);
      el("convertMeta").textContent = "Conversion failed";
    } finally {
      convertBtn.disabled = false;
      if (editors.layout) editors.layout();
    }
  });

  compileBtn.addEventListener("click", async () => {
    compileBtn.disabled = true;
    el("pdfMeta").textContent = "Compiling...";
    try {
      const title = el("title").value || "document";
      const response = await postJSON("/generate-pdf", {
        latex_code: editors.getLatex(),
        title
      });

      if (!response.ok) {
        const errData = await response.json();
        appendOutput("Compile failed: " + JSON.stringify(errData));
        el("pdfMeta").textContent = "Compilation failed";
        return;
      }

      const blob = await response.blob();
      if (lastPDFBlobURL) {
        URL.revokeObjectURL(lastPDFBlobURL);
      }
      lastPDFBlobURL = URL.createObjectURL(blob);
      el("preview").src = lastPDFBlobURL;
      el("pdfMeta").textContent = `PDF generated (${blob.size} bytes)`;
      appendOutput("PDF compiled successfully.");
    } catch (error) {
      appendOutput("Compile error: " + error);
      el("pdfMeta").textContent = "Compilation failed";
    } finally {
      compileBtn.disabled = false;
    }
  });

  overleafBtn.addEventListener("click", () => {
    if (!lastConvertSessionID) {
      appendOutput("Convert first to generate an Overleaf session ZIP.");
      return;
    }
    const title = el("title").value || "Wiki Document";
    const url = buildOverleafURL(lastConvertSessionID, title);
    window.open(url, "_blank");
    appendOutput("Opened Overleaf with session: " + lastConvertSessionID);
  });

  if (!el("date").value) {
    el("date").value = currentDateISO();
  }
}

async function mountEdit() {
  const convertBtn = el("convertBtn");
  const compileBtn = el("compileBtn");
  const overleafBtn = el("overleafBtn");
  const meta = el("meta");

  if (!convertBtn || !compileBtn || !overleafBtn || !meta || !queryParam("session_id")) {
    return;
  }

  const editors = await initMonacoEditors();
  let lastLatex = "";
  let lastZipSessionID = "";
  let lastPDFBlobURL = "";
  const sessionID = queryParam("session_id");

  meta.textContent = `Session ID: ${sessionID}`;

  try {
    const { response, data } = await getJSON("/api/sessions/" + encodeURIComponent(sessionID));
    if (!response.ok) {
      appendOutput("Failed loading session: " + JSON.stringify(data));
    } else if (data.page) {
      editors.setMarkdown(data.page.content || "");
      el("author").value = data.page.authorName || "";
      el("title").value = data.page.title || "";
      const settings = data.settings || {};
      el("date").value = settings.date || "";
      el("documentId").value = settings.documentId || "";
      el("template").value = settings.template || "space-race";
      el("footerText").value = settings.footerText || "";
      el("lineNumbersEnabled").checked = Boolean(settings.lineNumbersEnabled);
    }
  } catch (error) {
    appendOutput("Session load error: " + error);
  }

  if (!el("date").value) {
    el("date").value = currentDateISO();
  }

  convertBtn.addEventListener("click", async () => {
    convertBtn.disabled = true;
    try {
      const response = await postJSON("/convert", {
        markdown: editors.getMarkdown(),
        author: el("author").value,
        date: el("date").value || currentDateISO(),
        title: el("title").value,
        documentId: el("documentId").value,
        footerText: el("footerText").value,
        template: el("template").value || "space-race",
        lineNumbersEnabled: el("lineNumbersEnabled").checked
      });
      const data = await response.json();
      if (!response.ok) {
        appendOutput("Convert failed: " + JSON.stringify(data));
        return;
      }

      lastLatex = data.latex || "";
      lastZipSessionID = data.session_id || "";
      editors.setLatex(lastLatex);
      appendOutput("Convert success. Zip session: " + lastZipSessionID);
    } catch (error) {
      appendOutput("Convert error: " + error);
    } finally {
      convertBtn.disabled = false;
      if (editors.layout) editors.layout();
    }
  });

  compileBtn.addEventListener("click", async () => {
    compileBtn.disabled = true;
    try {
      const response = await postJSON("/generate-pdf", {
        latex_code: lastLatex || editors.getLatex(),
        title: el("title").value || "document"
      });
      if (!response.ok) {
        appendOutput("Compile failed: " + JSON.stringify(await response.json()));
        return;
      }
      const blob = await response.blob();
      if (lastPDFBlobURL) {
        URL.revokeObjectURL(lastPDFBlobURL);
      }
      lastPDFBlobURL = URL.createObjectURL(blob);
      el("preview").src = lastPDFBlobURL;
      appendOutput("Compile success.");
    } catch (error) {
      appendOutput("Compile error: " + error);
    } finally {
      compileBtn.disabled = false;
    }
  });

  overleafBtn.addEventListener("click", () => {
    if (!lastZipSessionID) {
      appendOutput("Convert first to create Overleaf ZIP session.");
      return;
    }
    const title = el("title").value || "Wiki Document";
    window.open(buildOverleafURL(lastZipSessionID, title), "_blank");
  });
}

mountIndex();
mountEdit();
