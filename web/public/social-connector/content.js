(() => {
  const FROM_PAGE = new Set(["NIMBUS_SOCIAL_CONNECT", "NIMBUS_SOCIAL_CONNECTOR_PING"]);

  function postToPage(message) {
    window.postMessage(message, window.location.origin);
  }

  postToPage({ type: "NIMBUS_SOCIAL_CONNECTOR_READY" });

  window.addEventListener("message", (event) => {
    if (event.source !== window) return;
    const message = event.data;
    if (!message || typeof message !== "object" || !FROM_PAGE.has(message.type)) return;

    if (message.type === "NIMBUS_SOCIAL_CONNECTOR_PING") {
      postToPage({ type: "NIMBUS_SOCIAL_CONNECTOR_READY" });
      return;
    }

    chrome.runtime.sendMessage(message, (response) => {
      const error = chrome.runtime.lastError;
      if (error) {
        postToPage({
          type: "NIMBUS_SOCIAL_CONNECT_RESULT",
          provider: message.provider,
          ok: false,
          error: error.message || "Nimbus Social Connector could not read the browser session.",
        });
        return;
      }
      postToPage({
        type: "NIMBUS_SOCIAL_CONNECT_RESULT",
        provider: message.provider,
        ...response,
      });
    });
  });
})();
