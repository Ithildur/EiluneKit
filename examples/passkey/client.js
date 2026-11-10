// Serve this module from the application's HTTPS origin (localhost works for development).
// 从应用的 HTTPS origin 提供此模块（开发时可使用 localhost）。
async function exchange(endpoint, body, accessToken) {
  const headers = { "Content-Type": "application/json" };
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  const response = await fetch(endpoint, {
    method: "POST",
    credentials: "same-origin",
    headers,
    body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error(`Passkey request failed: ${response.status}`);
  return response.json();
}

export async function registerPasskey(accessToken, name, endpoint = "/auth/passkey") {
  const begin = await exchange(endpoint, { operation: "register.begin" }, accessToken);
  const credential = await navigator.credentials.create({
    publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(begin.options.publicKey),
  });
  if (!credential) throw new Error("Passkey registration cancelled");
  return exchange(endpoint, {
    operation: "register.finish",
    flow_id: begin.flow_id,
    name,
    credential: credential.toJSON(),
  }, accessToken);
}

export async function loginWithPasskey(persistence = "session", endpoint = "/auth/passkey") {
  const begin = await exchange(endpoint, { operation: "login.begin", persistence });
  const credential = await navigator.credentials.get({
    publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(begin.options.publicKey),
  });
  if (!credential) throw new Error("Passkey login cancelled");
  return exchange(endpoint, {
    operation: "login.finish",
    flow_id: begin.flow_id,
    credential: credential.toJSON(),
  });
}
