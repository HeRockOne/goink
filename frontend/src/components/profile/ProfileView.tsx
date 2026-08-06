import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useApp } from '@/hooks/useApp'
import ContributionGrid from './ContributionGrid'
import { PenLine, CalendarDays, Flame, User, Camera, TrendingUp } from 'lucide-react'
import type { config } from '@/lib/wailsjs/go/models'

interface WritingStats {
  total_words: number
  total_days_active: number
  current_streak: number
  longest_streak: number
  total_novels: number
  total_chapters: number
}

interface DailyTokenUsage {
  date: string
  hit_tokens: number
  miss_tokens: number
  completion: number
  cost: number
  model: string
}

export default function ProfileView() {
  const app = useApp()
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activity, setActivity] = useState<Record<string, number>>({})
  const [stats, setStats] = useState<WritingStats | null>(null)
  const [settings, setSettings] = useState<config.AppSettings | null>(null)
  const [avatarKey, setAvatarKey] = useState(0)
  const [editingName, setEditingName] = useState(false)
  const [nameDraft, setNameDraft] = useState('')
  const [avatarErrored, setAvatarErrored] = useState(false)
  const [avatarError, setAvatarError] = useState('')
  const [nameError, setNameError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [currentYear] = useState(() => new Date().getFullYear())
  const [tokenTrend, setTokenTrend] = useState<DailyTokenUsage[]>([])
  const [selectedTrendModel, setSelectedTrendModel] = useState<string>('__all__')
  const [trendStart, setTrendStart] = useState(() => {
    const d = new Date(); d.setDate(d.getDate() - 30); return d.toISOString().slice(0, 10)
  })
  const [trendEnd, setTrendEnd] = useState(() => new Date().toISOString().slice(0, 10))

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [act, st, cfg] = await Promise.all([
        app.GetWritingActivity(12),
        app.GetWritingStats(),
        app.GetSettings(),
      ])
      const dict: Record<string, number> = {}
      if (act) {
        for (const d of act as Array<{ date: string; words: number }>) {
          dict[d.date] = d.words
        }
      }
      setActivity(dict)
      setStats(st as WritingStats)
      setSettings(cfg as config.AppSettings)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('profile.loadFailed'))
    } finally {
      setLoading(false)
    }
  }, [app, t])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    const days = Math.ceil((new Date(trendEnd).getTime() - new Date(trendStart).getTime()) / 86400000) + 1
    if (days < 1) return
    ;(app as any).GetTokenUsageTrend(days).then((data: DailyTokenUsage[]) => {
      const filtered = data.filter(d => d.date >= trendStart && d.date <= trendEnd)
      setTokenTrend(filtered)
    }).catch((err: any) => console.error('GetTokenUsageTrend failed', err))
  }, [app, trendStart, trendEnd])

  function handleAvatarClick() {
    fileInputRef.current?.click()
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    e.target.value = ''
    try {
      const buf = await file.arrayBuffer()
      await app.SaveAvatar(Array.from(new Uint8Array(buf)))
      setAvatarErrored(false)
      setAvatarKey(prev => prev + 1)
      setAvatarError('')
    } catch (err) {
      setAvatarError(err instanceof Error ? err.message : String(err))
    }
  }

  async function handleNameSave() {
    const name = nameDraft.trim()
    if (name && name !== settings?.user_name) {
      try {
        await app.SaveUserName(name)
        setSettings(prev => prev ? { ...prev, user_name: name } : null)
        setNameError('')
      } catch (err) {
        setNameError(err instanceof Error ? err.message : String(err))
        return
      }
    }
    setEditingName(false)
  }

  function startEditName() {
    setNameDraft(settings?.user_name ?? '')
    setEditingName(true)
  }

  if (loading) {
    return (
      <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">{t('profile.loading')}</div>
      </main>
    )
  }

  if (error) {
    return (
      <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
        <div className="flex h-full items-center justify-center text-sm text-rose-500">{error}</div>
      </main>
    )
  }

  return (
    <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      <input
        ref={fileInputRef}
        type="file" accept="image/*"
        className="hidden"
        onChange={handleFileChange}
      />
      <div className="max-w-4xl mx-auto px-6 py-8 space-y-8">
        {/* 头像 + 问候 */}
        <div className="flex items-center gap-4">
          <div className="relative group flex-shrink-0 cursor-pointer select-none" onClick={handleAvatarClick}>
            {avatarErrored ? (
              <div className="w-14 h-14 rounded-full bg-muted bg-secondary flex items-center justify-center">
                <User className="w-7 h-7 text-muted-foreground" />
              </div>
            ) : (
              <img
                src={`/avatar?v=${avatarKey}`}
                alt=""
                onError={() => setAvatarErrored(true)}
                className="w-14 h-14 rounded-full object-cover"
              />
            )}
            <div className="absolute inset-0 rounded-full flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity">
              <Camera className="w-5 h-5 text-white" />
            </div>
          </div>
          {avatarError && <p className="text-xs text-destructive">{avatarError}</p>}
          <div>
            {editingName ? (
              <div>
                <input
                  autoFocus
                  value={nameDraft}
                  onChange={e => { setNameDraft(e.target.value); setNameError('') }}
                  onBlur={handleNameSave}
                  onKeyDown={e => { if (e.key === 'Enter') handleNameSave(); if (e.key === 'Escape') setEditingName(false) }}
                  className="text-lg font-semibold bg-transparent border-b border-primary outline-none text-foreground max-w-[200px]"
                />
                {nameError && <p className="text-xs text-destructive mt-0.5">{nameError}</p>}
              </div>
            ) : (
              <h1
                onClick={startEditName}
                className={`text-lg font-semibold cursor-pointer hover:text-primary transition-colors select-none ${settings?.user_name ? 'text-foreground' : 'text-muted-foreground'}`}
              >
                {settings?.user_name || t('profile.noNickname')}
              </h1>
            )}
            <p className="text-xs text-muted-foreground mt-0.5">
              {t('profile.pastYearStats', { count: Object.keys(activity).length })}
            </p>
          </div>
        </div>

        {/* 统计卡片 */}
        <div className="grid grid-cols-2 gap-3">
          <StatCard
            icon={PenLine}
            label={t('profile.totalWords')}
            value={(stats?.total_words ?? 0).toLocaleString()}
          />
          <StatCard
            icon={CalendarDays}
            label={t('profile.writingDays')}
            value={`${stats?.total_days_active ?? 0}`}
          />
          <StatCard
            icon={Flame}
            label={t('profile.streakDays')}
            value={`${stats?.current_streak ?? 0} ${t('profile.day')}`}
          />
          <StatCard
            icon={Flame}
            label={t('profile.longestStreak')}
            value={`${stats?.longest_streak ?? 0} ${t('profile.day')}`}
          />
        </div>

        {/* Token 消耗趋势 */}
        <section>
          <h2 className="text-sm font-medium text-foreground mb-4 flex items-center gap-2">
            <TrendingUp className="w-4 h-4 text-muted-foreground" />
            {t('profile.tokenTrend')}
          </h2>

          <div className="flex items-center gap-2 mb-3">
            <input type="date" value={trendStart} onChange={e => setTrendStart(e.target.value)}
              className="h-7 rounded-md border bg-background px-2 text-xs outline-none w-[130px]" />
            <span className="text-xs text-muted-foreground">~</span>
            <input type="date" value={trendEnd} onChange={e => setTrendEnd(e.target.value)}
              className="h-7 rounded-md border bg-background px-2 text-xs outline-none w-[130px]" />
            <select value={selectedTrendModel} onChange={e => setSelectedTrendModel(e.target.value)}
              className="h-7 rounded-md border bg-background px-2 text-xs outline-none flex-1">
              <option value="__all__">全部模型</option>
              {tokenTrend.length > 0 && [...new Set(tokenTrend.map(d => d.model))].map(m => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </div>

          <div className="rounded-lg border bg-card p-4">
            {tokenTrend.length === 0 ? (
              <p className="text-xs text-muted-foreground text-center py-8">暂无数据</p>
            ) : (() => {
              const data = selectedTrendModel === '__all__' ? tokenTrend : tokenTrend.filter(d => d.model === selectedTrendModel)
              if (data.length === 0) return <p className="text-xs text-muted-foreground text-center py-8">暂无数据</p>
              const totalHit = data.reduce((s, d) => s + d.hit_tokens, 0)
              const totalMiss = data.reduce((s, d) => s + d.miss_tokens, 0)
              const totalComp = data.reduce((s, d) => s + d.completion, 0)
              const totalCost = data.reduce((s, d) => s + d.cost, 0)
              return <>
                <div className="grid grid-cols-4 gap-3 mb-4">
                  <StatCard icon={TrendingUp} label={t('profile.monthTotal')} value={((totalHit + totalMiss + totalComp) / 1_000_000).toFixed(2) + 'M Token'} />
                  <StatCard icon={TrendingUp} label={t('profile.monthCost')} value={'¥' + totalCost.toFixed(2)} />
                  <StatCard icon={TrendingUp} label={t('profile.cacheHitRate')} value={totalHit + totalMiss > 0 ? (totalHit / (totalHit + totalMiss) * 100).toFixed(1) + '%' : '0%'} />
                  <StatCard icon={TrendingUp} label={t('profile.cacheRead')} value={(totalHit / 1_000_000).toFixed(2) + 'M Token'} />
                </div>
                <div className="flex justify-center py-4">
                  <svg width="160" height="160" viewBox="0 0 160 160">
                    {(() => {
                      const total = totalHit + totalMiss + totalComp
                      if (total === 0) return <text x="80" y="80" textAnchor="middle" fill="var(--muted-foreground)" fontSize="12">无数据</text>
                      const pct = (v: number) => v / total * 360
                      const hA = pct(totalHit)
                      const mA = pct(totalMiss)
                      const cA = pct(totalComp)
                      const rad = (deg: number) => (deg - 90) * Math.PI / 180
                      const arc = (r: number, start: number, end: number) => {
                        const s = rad(start), e = rad(end)
                        const x1 = 80 + r * Math.cos(s), y1 = 80 + r * Math.sin(s)
                        const x2 = 80 + r * Math.cos(e), y2 = 80 + r * Math.sin(e)
                        const large = end - start > 180 ? 1 : 0
                        return `M 80 80 L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`
                      }
                      let start = 0
                      const slices: { color: string; label: string; val: number; deg: number }[] = []
                      if (totalHit > 0) { slices.push({ color: 'var(--chart-1, #52c41a)', label: '缓存命中', val: totalHit, deg: hA }) }
                      if (totalMiss > 0) { slices.push({ color: 'var(--chart-2, #f59e0b)', label: '未命中', val: totalMiss, deg: mA }) }
                      if (totalComp > 0) { slices.push({ color: 'var(--chart-3, #ef4444)', label: '输出', val: totalComp, deg: cA }) }
                      return <>
                        {slices.map(s => {
                          const el = <path key={s.label} d={arc(60, start, start + s.deg)} fill={s.color} stroke="var(--card)" strokeWidth="1" />
                          start += s.deg
                          return el
                        })}
                        <circle cx="80" cy="80" r="35" fill="var(--card)" />
                        <text x="80" y="76" textAnchor="middle" fill="var(--foreground)" fontSize="16" fontWeight="bold">{total > 0 ? (totalHit / total * 100).toFixed(0) + '%' : '-'}</text>
                        <text x="80" y="92" textAnchor="middle" fill="var(--muted-foreground)" fontSize="10">缓存命中</text>
                      </>
                    })()}
                  </svg>
                </div>
                <div className="flex justify-center gap-6 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-sm" style={{ backgroundColor: 'var(--chart-1, #52c41a)' }} /> 缓存命中 {(totalHit / 1_000_000).toFixed(2)}M</span>
                  <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-sm" style={{ backgroundColor: 'var(--chart-2, #f59e0b)' }} /> 未命中 {(totalMiss / 1_000_000).toFixed(2)}M</span>
                  <span className="flex items-center gap-1.5"><span className="w-3 h-3 rounded-sm" style={{ backgroundColor: 'var(--chart-3, #ef4444)' }} /> 输出 {(totalComp / 1_000_000).toFixed(2)}M</span>
                </div>
              </>
            })()}
          </div>
        </section>

        {/* 作品/章节概览 */}
        <div className="flex gap-6 text-xs text-muted-foreground">
          <span>{t('profile.worksCount', { count: stats?.total_novels ?? 0 })}</span>
          <span>{t('profile.chaptersCount', { count: stats?.total_chapters ?? 0 })}</span>
        </div>

        {/* 绿格子 */}
        <section>
          <h2 className="text-sm font-medium text-foreground mb-4">
            {t('profile.yearCalendar', { year: currentYear })}
          </h2>
          <div className="overflow-x-auto">
            <ContributionGrid data={activity} />
          </div>
        </section>

        {Object.keys(activity).length === 0 && (
          <div className="text-center py-12">
            <PenLine className="w-10 h-10 mx-auto text-muted-foreground mb-3" />
            <p className="text-sm text-muted-foreground">
              {t('profile.noWritingRecord')}
            </p>
          </div>
        )}
      </div>
    </main>
  )
}

function StatCard({ icon: Icon, label, value }: { icon: any; label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-card px-4 py-3 space-y-1">
      <div className="flex items-center gap-1.5 text-muted-foreground">
        <Icon className="w-3.5 h-3.5" />
        <span className="text-[11px]">{label}</span>
      </div>
      <p className="text-lg font-semibold text-foreground">{value}</p>
    </div>
  )
}
