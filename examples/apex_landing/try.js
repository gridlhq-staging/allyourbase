const form = document.querySelector("#try-form");
const statusPanel = document.querySelector("#launch-status");
const submitButton = form?.querySelector("button[type=submit]");

window.turnstileReady = () => {
  submitButton.disabled = false;
};
window.turnstileExpired = () => {
  submitButton.disabled = true;
};

function showStatus(message, className = "") {
  statusPanel.hidden = false;
  statusPanel.replaceChildren();
  const paragraph = document.createElement("p");
  paragraph.textContent = message;
  paragraph.className = className;
  statusPanel.append(paragraph);
}

function showReady(result) {
  showStatus("Your private Allyourbase instance is ready.");
  const link = document.createElement("a");
  link.className = "launch-link";
  link.href = result.adminUrl;
  link.target = "_blank";
  link.rel = "noopener";
  link.textContent = "Open Allyourbase";
  statusPanel.append(link);

  const label = document.createElement("p");
  label.textContent = "Temporary admin password:";
  const password = document.createElement("code");
  password.className = "password";
  password.setAttribute("aria-label", "Temporary admin password");
  password.textContent = result.adminPassword;
  const expiration = document.createElement("p");
  expiration.textContent = `Expires ${new Date(result.expiresAt).toLocaleString()}`;
  statusPanel.append(label, password, expiration);
}

async function readJSON(response) {
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error ?? "The launch failed. Please try again.");
  return result;
}

async function pollLaunch(launchToken) {
  const result = await readJSON(await fetch(`/api/try/status?launch=${encodeURIComponent(launchToken)}`));
  if (result.status === "ready") {
    showReady(result);
    return;
  }
  window.setTimeout(() => pollLaunch(launchToken).catch(showError), 750);
}

function showError(error) {
  showStatus(error.message, "error");
  submitButton.disabled = false;
  submitButton.textContent = "Try again";
  window.turnstile?.reset?.();
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  submitButton.disabled = true;
  submitButton.textContent = "Launching…";
  showStatus("Starting your private Allyourbase instance…");
  try {
    const launch = await readJSON(await fetch(form.action, { method: "POST", body: new FormData(form) }));
    await pollLaunch(launch.launchToken);
  } catch (error) {
    showError(error);
  }
});
