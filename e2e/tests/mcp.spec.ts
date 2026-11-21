import { test, expect, APIRequestContext } from '@playwright/test'
import { MCP_OWNER_UID, MCP_TOKEN } from './helpers'

const MCP_URL = 'http://localhost:8080/mcp'
const MCP_HEADERS = {
  'Content-Type': 'application/json',
  Accept: 'application/json, text/event-stream',
}

let nextId = 1

async function rpc(request: APIRequestContext, method: string, params: object = {}) {
  const res = await request.post(MCP_URL, {
    headers: { ...MCP_HEADERS, Authorization: `Bearer ${MCP_TOKEN}` },
    data: { jsonrpc: '2.0', id: nextId++, method, params },
  })
  expect(res.status()).toBe(200)
  return (await res.json()).result
}

async function callTool(request: APIRequestContext, name: string, args: object = {}) {
  return rpc(request, 'tools/call', { name, arguments: args })
}

test('rejects requests without the MCP token', async ({ request }) => {
  const body = { jsonrpc: '2.0', id: 1, method: 'tools/list', params: {} }

  const none = await request.post(MCP_URL, { headers: MCP_HEADERS, data: body })
  expect(none.status()).toBe(401)

  const wrong = await request.post(MCP_URL, {
    headers: { ...MCP_HEADERS, Authorization: 'Bearer wrong' },
    data: body,
  })
  expect(wrong.status()).toBe(401)
})

test('practices a sentence end to end', async ({ request }) => {
  await rpc(request, 'initialize', {
    protocolVersion: '2025-06-18',
    capabilities: {},
    clientInfo: { name: 'e2e', version: '0.0.0' },
  })

  const list = await rpc(request, 'tools/list')
  expect(list.tools).toHaveLength(4)

  const practice = await callTool(request, 'get_practice_sentence')
  expect(practice.isError).toBeFalsy()
  const sentenceId: number = practice.structuredContent.id

  const answer = 'Definitely wrong from Muse'
  const check = await callTool(request, 'check_answer', { sentence_id: sentenceId, answer })
  expect(check.isError).toBeFalsy()
  expect(check.structuredContent.is_correct).toBe(false)

  const host = process.env.FIRESTORE_EMULATOR_HOST
  const doc = await request.get(
    `http://${host}/v1/projects/eagle-test/databases/(default)/documents/users/${MCP_OWNER_UID}/sentence_stats/${sentenceId}`,
    { headers: { Authorization: 'Bearer owner' } }
  )
  expect(doc.status()).toBe(200)
  const stats = await doc.json()
  expect(Number(stats.fields.incorrect_count.integerValue)).toBeGreaterThanOrEqual(1)

  const mistakes = await callTool(request, 'list_mistakes')
  const entry = mistakes.structuredContent.mistakes.find(
    (m: { sentence_id: number }) => m.sentence_id === sentenceId
  )
  expect(entry).toBeDefined()
  expect(entry.latest_wrong_answers.map((w: { answer: string }) => w.answer)).toContain(answer)
})

test('errors on unknown sentence ids', async ({ request }) => {
  const result = await callTool(request, 'report_sentence', { sentence_id: 999999 })
  expect(result.isError).toBe(true)
  expect(result.content[0].text).toContain('Sentence 999999 not found')
})
