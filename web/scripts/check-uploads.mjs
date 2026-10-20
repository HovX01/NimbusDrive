import assert from "node:assert/strict";
import { File } from "node:buffer";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const read = path => readFile(new URL(path, import.meta.url), "utf8");
const compile = source => ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, jsx: ts.JsxEmit.React, jsxFactory: "element" },
}).outputText;
const tree = ts.createSourceFile("App.tsx", await read("../src/App.tsx"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const statements = tree.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === "App").body.statements;
const declaration = name => {
  const node = statements.find(node => node.name?.text === name || (ts.isVariableStatement(node) && node.declarationList.declarations.some(d => d.name.getText(tree) === name)));
  assert(node, `Missing declaration: ${name}`);
  return node.getText(tree);
};
const effect = dependency => {
  const call = statements.filter(ts.isExpressionStatement).map(node => node.expression).find(node =>
    ts.isCallExpression(node) && node.expression.getText(tree) === "useEffect" && node.arguments[1]?.getText(tree).includes(dependency));
  assert(call, `Missing effect: ${dependency}`);
  return call.arguments[0].getText(tree);
};
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
};
const settle = () => new Promise(resolve => setImmediate(resolve));
const requests = [], lists = [], listeners = new Map();
let active = 0, maximum = 0;
const harness = new Function("uploadFile", "listFiles", "window", "pushToast", compile(`
  let uploads = [], token = "token-a", parentId = "folder-a", filesVersion = 0, items = [], error = "", busy = false;
  const runningUploads = { current: new Set() };
  const useCallback = fn => fn;
  const setUploads = update => { uploads = update(uploads); };
  const setFilesVersion = update => { filesVersion = update(filesVersion); };
  const setItems = value => { items = value; };
  const setError = value => { error = value; };
  const setBusy = value => { busy = value; };
  ${["patchUpload", "clearUploads", "dismissUpload", "retryUpload", "onUpload"].map(declaration).join("\n")}
  return {
    onUpload, retryUpload, clearUploads, dismissUpload,
    pump: ${effect("patchUpload")}, load: ${effect("filesVersion")},
    warn: () => { ${declaration("hasPendingUploads")} return (${effect("hasPendingUploads")})(); },
    folder: value => { parentId = value; }, session: value => { token = value; },
    get jobs() { return uploads; }, get version() { return filesVersion; },
    get items() { return items; }, get error() { return error; }, get busy() { return busy; }
  };
`))((...args) => {
  const request = { ...deferred(), args };
  requests.push(request);
  maximum = Math.max(maximum, ++active);
  return request.promise.then(value => { active--; return value; }, error => { active--; throw error; });
}, (...args) => {
  const request = { ...deferred(), args };
  lists.push(request);
  return request.promise;
}, {
  addEventListener: (name, handler) => listeners.set(name, handler),
  removeEventListener: (name, handler) => { if (listeners.get(name) === handler) listeners.delete(name); },
}, () => {});

const files = Array.from({ length: 21 }, (_, i) => new File(["test"], `file-${i}.txt`));
harness.onUpload(files);
harness.pump();
harness.pump(); // Development effect replay must not start duplicate requests.
assert.equal(requests.length, 3);
harness.clearUploads();
harness.dismissUpload(harness.jobs[0].id);
assert.equal(harness.jobs.length, 21, "Active and queued jobs must remain tracked");
const stopWarning = harness.warn();
const event = { prevented: false, preventDefault() { this.prevented = true; } };
listeners.get("beforeunload")(event);
assert(event.prevented);
assert.equal(event.returnValue, "");
stopWarning();
assert.equal(listeners.size, 0);

harness.folder("folder-b");
harness.onUpload([new File(["test"], "other-folder.txt")]);
harness.pump();
assert.equal(requests.length, 3, "Separate selections must share the concurrency limit");
assert.equal(harness.jobs.length, 22);
const failedId = harness.jobs[0].id;
requests[0].args[3](99);
requests[0].args[4]();
assert.equal(harness.jobs[0].status, "processing");
requests[0].reject(new Error("Offline"));
await settle();
assert.equal(harness.jobs[0].status, "error");
assert.equal(harness.jobs[0].file, files[0]);
harness.session("renewed-token");
harness.retryUpload(failedId);
harness.retryUpload(failedId);
assert.equal(harness.jobs[0].progress, 0);
assert.equal(harness.jobs[0].error, undefined);
harness.pump();
assert.equal(requests.length, 4);
assert.deepEqual(requests[3].args.slice(0, 3), ["renewed-token", "folder-a", files[0]]);
harness.pump();
assert.equal(requests.length, 4, "Repeated retry must not duplicate a running upload");

requests[1].resolve({ id: "saved" });
await settle();
assert.equal(harness.jobs[1].status, "done");
assert.equal(harness.jobs[1].file, undefined, "Saved files must release their retained File reference");
assert.equal(harness.version, 1);
const stopRefresh = harness.load();
assert.deepEqual(lists[0].args, ["renewed-token", "folder-b"]);
lists[0].reject(new Error("List refresh failed"));
await settle();
assert.equal(harness.error, "List refresh failed");
assert.equal(harness.jobs[1].status, "done", "Refresh errors must not turn saved files into failed uploads");
stopRefresh();
harness.pump();
for (let i = 2; i < requests.length; i++) {
  requests[i].resolve({ id: `saved-${i}` });
  await settle();
  harness.pump();
}
assert.equal(maximum, 3);
assert(harness.jobs.every(job => job.status === "done"));
assert.equal(requests.at(-1).args[1], "folder-b");
assert.equal(harness.warn(), undefined);
assert.equal(listeners.size, 0);
harness.clearUploads();
assert.equal(harness.jobs.length, 0);

harness.folder("folder-a");
const stopOldList = harness.load();
harness.folder("folder-b");
stopOldList();
const stopNewList = harness.load();
lists[2].resolve({ items: [{ id: "folder-b-file" }] });
await settle();
lists[1].resolve({ items: [{ id: "folder-a-file" }] });
await settle();
assert.deepEqual(harness.items, [{ id: "folder-b-file" }], "Late responses must not replace the current folder");
assert.equal(harness.busy, false);
stopNewList();

const apiSource = (await read("../src/api.ts")).replace('const API = import.meta.env.VITE_API_BASE ?? "";', 'const API = "";');
const api = await import(`data:text/javascript;base64,${Buffer.from(compile(apiSource)).toString("base64")}`);
globalThis.localStorage = { getItem: () => "access-key" };
let xhr;
globalThis.XMLHttpRequest = class {
  upload = {};
  headers = {};
  constructor() { xhr = this; }
  open(method, url) { this.method = method; this.url = url; }
  setRequestHeader(name, value) { this.headers[name] = value; }
  send(body) { this.body = body; }
};
let progress, processing = false;
const success = api.uploadFile("token", "folder", files[0], value => { progress = value; }, () => { processing = true; });
assert.equal(xhr.method, "POST");
assert.equal(xhr.url, "/api/v1/files/upload");
assert.equal(xhr.headers.Authorization, "Bearer token");
assert.equal(xhr.headers["X-Nimbus-Access"], "access-key");
assert.equal(xhr.body.get("parent_id"), "folder");
assert.equal(xhr.body.get("file").name, files[0].name);
xhr.upload.onprogress({ lengthComputable: true, loaded: 10, total: 10 });
assert.equal(progress, 99);
xhr.upload.onload();
assert.equal(progress, 100);
assert(processing);
xhr.status = 201;
xhr.responseText = '{"id":"saved","duplicate":true}';
xhr.onload();
assert.equal((await success).duplicate, true);
for (const body of ["null", "{}", "not JSON"]) {
  const result = api.uploadFile("token", "folder", files[0]);
  const rejected = assert.rejects(result, /Invalid upload response/);
  xhr.status = 200;
  xhr.responseText = body;
  xhr.onload();
  await rejected;
}
for (const [event, message] of [["onerror", /connection/], ["onabort", /cancelled/], ["ontimeout", /timed out/], ["onload", /Storage full/]]) {
  const result = api.uploadFile("token", "folder", files[0]);
  const rejected = assert.rejects(result, message);
  xhr.status = 507;
  xhr.responseText = '{"error":{"message":"Storage full"}}';
  xhr[event]();
  await rejected;
}

const panelSource = ts.createSourceFile("UploadPanel.tsx", await read("../src/components/UploadPanel.tsx"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const panelFunction = panelSource.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === "UploadPanel").getText(panelSource).replace(/^export /, "");
const context = {
  useEffect: () => {}, element: (type, props, ...children) => ({ type, props, children }),
  KIND_ICON: { file: "FileIcon" }, previewKind: () => "file", formatBytes: String, cn: String,
  ...Object.fromEntries(["Button", "Progress", "CircleAlert", "Check", "CloudUpload", "X", "Loader2", "Clock"].map(name => [name, name])),
};
const panel = new Function(...Object.keys(context), compile(`${panelFunction}\nreturn UploadPanel;`))(...Object.values(context));
const elements = node => !node || typeof node !== "object" ? [] : Array.isArray(node) ? node.flatMap(elements) : [node, ...node.children.flatMap(elements)];
let retried;
const render = jobs => elements(panel({ jobs, onClose() {}, onDismiss() {}, onRetry: id => { retried = id; } }));
const failedJob = { id: "failed", name: "failed.txt", size: 4, progress: 0, status: "error", error: "Offline" };
const failedPanel = render([failedJob]);
failedPanel.find(node => node.props?.["aria-label"] === "Retry failed.txt").props.onClick();
assert.equal(retried, "failed");
assert.equal(failedPanel.find(node => node.props?.["aria-label"] === "Close upload panel").props.disabled, false);
assert.equal(render([{ ...failedJob, status: "uploading" }]).find(node => node.props?.["aria-label"] === "Close upload panel").props.disabled, true);

console.log("Upload checks passed: shared queue, Retry, navigation, refresh errors, unload warning, XHR failures and panel controls");
