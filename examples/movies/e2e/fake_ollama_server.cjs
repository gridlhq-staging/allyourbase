const http = require("node:http");

const HOST = "127.0.0.1";
const PORT = 11434;

function readJSON(req, callback) {
  let raw = "";
  req.on("data", (chunk) => {
    raw += chunk.toString("utf8");
  });
  req.on("end", () => {
    if (!raw.trim()) {
      callback({});
      return;
    }
    try {
      callback(JSON.parse(raw));
    } catch {
      callback({});
    }
  });
}

function writeJSON(res, statusCode, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(statusCode, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(body),
  });
  res.end(body);
}

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/health") {
    writeJSON(res, 200, { ok: true });
    return;
  }

  if (req.method === "POST" && req.url === "/api/embed") {
    readJSON(req, (body) => {
      const input = Array.isArray(body.input) ? body.input : [""];
      const embeddings = input.map(() => [0.9, 0.1, 0.2]);
      writeJSON(res, 200, {
        model: body.model || "nomic-embed-text",
        embeddings,
      });
    });
    return;
  }

  if (req.method === "POST" && req.url === "/api/chat") {
    readJSON(req, (body) => {
      const messages = Array.isArray(body.messages) ? body.messages : [];
      const last = messages[messages.length - 1];
      const content = typeof last?.content === "string" ? last.content : "stub response";
      writeJSON(res, 200, {
        model: body.model || "llama3.2",
        message: {
          role: "assistant",
          content: `Local stub response: ${content}`,
        },
        done_reason: "stop",
        prompt_eval_count: 1,
        eval_count: 1,
      });
    });
    return;
  }

  writeJSON(res, 404, { error: "not found" });
});

server.listen(PORT, HOST, () => {
  process.stdout.write(`fake-ollama-listening:${HOST}:${PORT}\n`);
});
