export type AgentPlatform = 'windows' | 'linux' | 'macos' | 'android' | 'ios' | 'other'

export function detectAgentPlatform(userAgent: string, platform: string, touchPoints: number): AgentPlatform {
  if (/android/i.test(userAgent)) return 'android'
  if (/iphone|ipad|ipod/i.test(userAgent) || (/mac/i.test(platform) && touchPoints > 1)) return 'ios'
  if (/cros/i.test(userAgent)) return 'other'
  if (/win/i.test(platform) || /windows/i.test(userAgent)) return 'windows'
  if (/mac/i.test(platform) || /macintosh/i.test(userAgent)) return 'macos'
  if (/linux/i.test(platform) || /linux/i.test(userAgent)) return 'linux'
  return 'other'
}
