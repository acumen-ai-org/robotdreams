export type SSEHandler = (eventType: string, data: unknown) => void;

export async function readSSE(resp: Response, onEvent: SSEHandler): Promise<void> {
  if (!resp.body) return;
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let partialLine = "";
  let curEvent = "message";

  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    partialLine += decoder.decode(value, { stream: true });
    const lines = partialLine.split("\n");
    partialLine = lines.pop() ?? "";

    for (const line of lines) {
      if (line.startsWith("event: ")) {
        curEvent = line.slice(7).trim();
      } else if (line.startsWith("data: ")) {
        const payload = parseJSON(line.slice(6));
        if (payload !== undefined) onEvent(curEvent, payload);
      } else if (line === "") {
        curEvent = "message";
      }
    }
  }
}

function parseJSON(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return undefined;
  }
}
