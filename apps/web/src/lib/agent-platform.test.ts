import { expect, it } from 'vitest'
import { detectAgentPlatform } from './agent-platform'

it('recognizes the endpoint OS without assuming a CPU architecture', () => {
  expect(detectAgentPlatform('Windows NT 10.0', '', 0)).toBe('windows')
  expect(detectAgentPlatform('X11; Linux x86_64', '', 0)).toBe('linux')
  expect(detectAgentPlatform('Macintosh; Intel Mac OS X', 'MacIntel', 0)).toBe('macos')
  expect(detectAgentPlatform('Linux; Android 16', 'Linux armv8l', 5)).toBe('android')
  expect(detectAgentPlatform('Macintosh; Intel Mac OS X', 'MacIntel', 5)).toBe('ios')
  expect(detectAgentPlatform('X11; CrOS x86_64', 'Linux x86_64', 0)).toBe('other')
})
