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

function writeOllamaStreamLine(res, payload) {
  res.write(`${JSON.stringify(payload)}\n`);
}

function streamOllamaChat(res, body, content) {
  const model = body.model || "llama3.2";
  res.writeHead(200, {
    "content-type": "application/x-ndjson",
    "cache-control": "no-cache",
    connection: "keep-alive",
  });

  const [firstWord, ...rest] = content.split(" ");
  const chunks = [
    "Local stub response:",
    ` ${firstWord}`,
    rest.length > 0 ? ` ${rest.join(" ")}` : "",
  ].filter((chunk) => chunk.length > 0);
  chunks.forEach((chunk, index) => {
    setTimeout(() => {
      writeOllamaStreamLine(res, {
        model,
        message: { role: "assistant", content: chunk },
        done: false,
      });
      if (index === chunks.length - 1) {
        writeOllamaStreamLine(res, { model, done: true, done_reason: "stop" });
        res.end();
      }
    }, 150 * (index + 1));
  });
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

  if (req.method === "POST" && req.url === "/v1/embeddings") {
    readJSON(req, (body) => {
      const input = Array.isArray(body.input) ? body.input : [body.input ?? ""];
      const data = input.map(() => ({ embedding: [0.9, 0.1, 0.2] }));
      writeJSON(res, 200, {
        object: "list",
        model: body.model || "text-embedding-3-small",
        data,
        usage: { prompt_tokens: 1, total_tokens: 1 },
      });
    });
    return;
  }

  if (req.method === "POST" && req.url === "/api/chat") {
    readJSON(req, (body) => {
      const messages = Array.isArray(body.messages) ? body.messages : [];
      const last = messages[messages.length - 1];
      const content = typeof last?.content === "string" ? last.content : "stub response";
      if (body.stream === true) {
        streamOllamaChat(res, body, content);
        return;
      }
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

  if (req.method === "POST" && req.url === "/v1/chat/completions") {
    readJSON(req, (body) => {
      const messages = Array.isArray(body.messages) ? body.messages : [];
      const last = messages[messages.length - 1];
      const content = typeof last?.content === "string" ? last.content : "stub response";
      writeJSON(res, 200, {
        id: "chatcmpl-demo",
        object: "chat.completion",
        model: body.model || "gpt-4o-mini",
        choices: [
          {
            index: 0,
            message: { role: "assistant", content: `Local stub response: ${content}` },
            finish_reason: "stop",
          },
        ],
        usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
      });
    });
    return;
  }

  writeJSON(res, 404, { error: "not found" });
});

server.listen(PORT, HOST, () => {
  process.stdout.write(`fake-ollama-listening:${HOST}:${PORT}\n`);
});
