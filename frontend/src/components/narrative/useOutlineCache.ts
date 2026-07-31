import { useCallback, useRef } from 'react'
import { useApp } from '@/hooks/useApp'

export function useOutlineCache() {
  const app = useApp()
  const cacheRef = useRef<Record<number, string>>({})

  const loadOutlines = useCallback(async (novelId: number, chapterNum: number): Promise<string | null> => {
    if (chapterNum <= 0 || !novelId) return null

    const cached = cacheRef.current[chapterNum]
    if (cached !== undefined) return cached || null

    try {
      const path = `outlines/${String(chapterNum).padStart(3, '0')}.md`
      const content = await app.GetContent(novelId, path)
      cacheRef.current[chapterNum] = content || ''
      return content || null
    } catch {
      cacheRef.current[chapterNum] = ''
      return null
    }
  }, [app])

  const invalidateCache = useCallback((chapterNum?: number) => {
    if (chapterNum !== undefined) {
      delete cacheRef.current[chapterNum]
    } else {
      cacheRef.current = {}
    }
  }, [])

  return {
    loadOutlines,
    invalidateCache,
  }
}
