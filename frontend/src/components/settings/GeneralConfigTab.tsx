import { useState, useEffect, useRef } from 'react'
import { Folder, RefreshCw, GitFork, Languages, Shield, Wifi, WifiOff, Archive, RotateCcw, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import { SaveGitConfig, SetPhaseGateEnabled, SetChapterWordLimit } from '@/lib/wailsjs/go/app/App'
import { useApp, type novel } from '@/hooks/useApp'

export default function GeneralConfigTab() {
  const app = useApp()
  const { t, i18n } = useTranslation()
  const [dataDir, setDataDir] = useState('')
  const [novels, setNovels] = useState<novel.Novel[]>([])
  const [selectedID, setSelectedID] = useState<number>(0)
  const [rebuilding, setRebuilding] = useState(false)
  const [gitName, setGitName] = useState('')
  const [gitEmail, setGitEmail] = useState('')
  const [gitSaving, setGitSaving] = useState(false)
  const [gitSaved, setGitSaved] = useState(false)
  const [gitError, setGitError] = useState<string | null>(null)
  const [phaseGateEnabled, setPhaseGateEnabled] = useState(true)
  const [webdavRunning, setWebdavRunning] = useState(false)
  const [webdavPort, setWebdavPort] = useState('12345')
  const [webdavUser, setWebdavUser] = useState('1')
  const [webdavPass, setWebdavPass] = useState('1')
  const [webdavInfo, setWebdavInfo] = useState('')
  const [minChapterWords, setMinChapterWords] = useState('2500')
  const [maxChapterWords, setMaxChapterWords] = useState('4000')
  const [backing, setBacking] = useState(false)
  const [backupPath, setBackupPath] = useState('')
  const [backupError, setBackupError] = useState('')
  const [restoring, setRestoring] = useState(false)
  const [restoreMsg, setRestoreMsg] = useState('')
  const restoreInputRef = useRef<HTMLInputElement>(null)
  const [apiPort, setApiPort] = useState('9323')
  const [apiToken, setApiToken] = useState('')
  const [loggingEnabled, setLoggingEnabled] = useState(true)

  useEffect(() => {
    app.GetAppConfig().then(cfg => {
      setDataDir((cfg?.data_dir as string) || '')
    }).catch(() => {})
    app.GetNovels().then(list => {
      setNovels(list || [])
    }).catch(() => {})
    // 获取 API token
    app.GetAPIToken().then(token => {
      if (token) setApiToken(token)
    }).catch(() => {})
    app.GetSettings().then(s => {
      if (s?.last_novel_id) setSelectedID(s.last_novel_id)
      if (s?.git_name) setGitName(s.git_name)
      if (s?.git_email) setGitEmail(s.git_email)
      if (s?.phase_gate_enabled !== undefined && s?.phase_gate_enabled !== null) {
        setPhaseGateEnabled(s.phase_gate_enabled as boolean)
      }
      if (s?.webdav_port) setWebdavPort(String(s.webdav_port))
      if (s?.webdav_user) setWebdavUser(s.webdav_user)
      if (s?.webdav_pass) setWebdavPass(s.webdav_pass)
      if (s?.min_chapter_words) setMinChapterWords(String(s.min_chapter_words))
      if (s?.max_chapter_words) setMaxChapterWords(String(s.max_chapter_words))
      if (s?.api_port) setApiPort(String(s.api_port))
      if (s?.log_enabled !== undefined && s?.log_enabled !== null) {
        setLoggingEnabled(s.log_enabled as boolean)
      }
    }).catch(() => {})
    app.GetLoggingEnabled().then(v => setLoggingEnabled(v)).catch(() => {})
    app.IsWebDAVRunning().then(r => setWebdavRunning(r)).catch(() => {})
    app.GetWebDAVInfo().then(i => setWebdavInfo(i)).catch(() => {})
  }, [app])

  async function handleSaveGit() {
    setGitSaving(true)
    setGitSaved(false)
    setGitError(null)
    try {
      await SaveGitConfig(gitName, gitEmail)
      setGitSaved(true)
      setTimeout(() => setGitSaved(false), 2000)
    } catch (err) {
      setGitError(err instanceof Error ? err.message : t('settings.saveFailed'))
    } finally {
      setGitSaving(false)
    }
  }

  async function handleRebuild() {
    if (!selectedID) return
    setRebuilding(true)
    try {
      await app.RebuildNovelIndex(selectedID)
    } catch (err) {
      console.error('Rebuild failed:', err)
    } finally {
      setRebuilding(false)
    }
  }

  async function handlePhaseGateToggle() {
    const newValue = !phaseGateEnabled
    setPhaseGateEnabled(newValue)
    try {
      await SetPhaseGateEnabled(newValue)
    } catch (err) {
      setPhaseGateEnabled(!newValue)
      console.error('Failed to save phase gate setting:', err)
    }
  }

  async function handleSaveWordLimit() {
    const min = parseInt(minChapterWords) || 2500
    const max = parseInt(maxChapterWords) || 4000
    try {
      await SetChapterWordLimit(min, max)
    } catch (err) {
      console.error('Failed to save word limit:', err)
    }
  }

  async function handleSaveAPIPort() {
    const port = parseInt(apiPort) || 9323
    try {
      await app.SetAPIPort(port)
    } catch (err) {
      console.error('Failed to save API port:', err)
    }
  }

  async function handleResetToken() {
    try {
      const token = await app.ResetAPIToken()
      if (token) setApiToken(token)
    } catch (err) {
      console.error('Failed to reset token:', err)
    }
  }

  async function handleBackup() {
    setBacking(true)
    setBackupError('')
    setBackupPath('')
    try {
      const path = await app.BackupData()
      setBackupPath(path)
    } catch (err) {
      setBackupError(err instanceof Error ? err.message : '备份失败')
    } finally {
      setBacking(false)
    }
  }

  async function handleRestore(file: File) {
    setRestoring(true)
    setRestoreMsg('')
    try {
      // 通过 Wails 读取文件内容并写入临时路径
      const arrayBuffer = await file.arrayBuffer()
      const uint8 = new Uint8Array(arrayBuffer)
      // 将文件写入数据目录
      const tempPath = await app.WriteTempFile(file.name, Array.from(uint8))
      await app.RestoreData(tempPath)
      setRestoreMsg('恢复成功！请重启应用使数据生效。')
    } catch (err) {
      setRestoreMsg('恢复失败: ' + (err instanceof Error ? err.message : String(err)))
    } finally {
      setRestoring(false)
    }
  }

  async function toggleWebDAV() {
    try {
      if (webdavRunning) {
        await app.StopWebDAV()
        setWebdavRunning(false)
        setWebdavInfo('')
      } else {
        const port = parseInt(webdavPort) || 12345
        await app.SetWebDAVConfig(port, webdavUser, webdavPass)
        await app.StartWebDAV(port)
        setWebdavRunning(true)
        const info = await app.GetWebDAVInfo()
        setWebdavInfo(info)
      }
    } catch (e) {
      console.error('WebDAV toggle failed', e)
    }
  }

  async function handleLoggingToggle() {
    const newValue = !loggingEnabled
    setLoggingEnabled(newValue)
    try {
      await app.SetLoggingEnabled(newValue)
    } catch (err) {
      setLoggingEnabled(!newValue)
      console.error('Failed to toggle logging:', err)
    }
  }

  return (
    <div className="flex-1 flex flex-col overflow-y-auto">
      <h3 className="text-sm font-medium mb-5">{t('settings.basicConfig')}</h3>

      <div className="space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Folder className="w-3.5 h-3.5" />
          {t('settings.dataDir')}
        </label>
        <div className="flex items-center gap-2">
          <input
            value={dataDir}
            readOnly
            className="flex-1 h-8 rounded-md border bg-muted/50 px-3 text-xs font-mono focus:outline-none cursor-default"
          />
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Shield className="w-3.5 h-3.5" />
          {t('settings.phaseGate')}
        </label>
        <p className="text-[11px] text-muted-foreground">{t('settings.phaseGateDesc')}</p>
        <button
          onClick={handlePhaseGateToggle}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
            phaseGateEnabled ? 'bg-primary' : 'bg-muted'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              phaseGateEnabled ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>

      {/* 章节字数范围 */}
      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Shield className="w-3.5 h-3.5" />
          章节字数范围
        </label>
        <p className="text-[11px] text-muted-foreground">AI 写完章节后自动校验字数，不达标会自动修复</p>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-12 shrink-0">最少</span>
          <input
            value={minChapterWords}
            onChange={e => setMinChapterWords(e.target.value)}
            type="number"
            min="100"
            max="10000"
            className="w-24 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
          />
          <span className="text-xs text-muted-foreground">字</span>
          <span className="text-xs text-muted-foreground mx-1">~</span>
          <span className="text-xs text-muted-foreground w-12 shrink-0">最多</span>
          <input
            value={maxChapterWords}
            onChange={e => setMaxChapterWords(e.target.value)}
            type="number"
            min="100"
            max="20000"
            className="w-24 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
          />
          <span className="text-xs text-muted-foreground">字</span>
          <button
            onClick={handleSaveWordLimit}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors"
          >
            保存
          </button>
        </div>
      </div>

      {/* 移动端连接端口 */}
      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Wifi className="w-3.5 h-3.5" />
          移动端连接端口
        </label>
        <p className="text-[11px] text-muted-foreground">手机 App 连接此端口。修改后需重启 Goink 生效。</p>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-16 shrink-0">API 端口</span>
          <input
            value={apiPort}
            onChange={e => setApiPort(e.target.value)}
            type="number"
            min="1024"
            max="65535"
            className="w-24 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
          />
          <button
            onClick={handleSaveAPIPort}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors"
          >
            保存
          </button>
        </div>
      </div>

      {/* API 认证令牌 */}
      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Shield className="w-3.5 h-3.5" />
          API 认证令牌
        </label>
        <p className="text-[11px] text-muted-foreground">移动端首次连接时需输入此令牌，或扫描下方二维码。丢失可重新生成。</p>
        <div className="flex items-start gap-4">
          <div className="flex-1 space-y-2">
            <div className="flex items-center gap-2">
              <input
                value={apiToken}
                readOnly
                className="flex-1 h-8 rounded-md border bg-background px-3 text-xs font-mono focus:outline-none"
              />
              <button
                onClick={() => { navigator.clipboard.writeText(apiToken) }}
                className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors"
              >
                复制
              </button>
              <button
                onClick={handleResetToken}
                className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors text-destructive"
              >
                重置
              </button>
            </div>
          </div>
          {apiToken && (
            <div className="shrink-0 p-2 bg-white rounded-lg border">
              <QRCodeSVG value={apiToken} size={80} level="M" />
            </div>
          )}
        </div>
      </div>

      {/* WebDAV 局域网阅读 */}
      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Wifi className="w-3.5 h-3.5" />
          WebDAV 局域网阅读
        </label>
        <p className="text-[11px] text-muted-foreground">手机文件管理器连接后可阅读小说（只读模式）</p>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">端口</span>
            <input
              value={webdavPort}
              onChange={e => setWebdavPort(e.target.value)}
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
              disabled={webdavRunning}
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">用户名</span>
            <input
              value={webdavUser}
              onChange={e => setWebdavUser(e.target.value)}
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
              disabled={webdavRunning}
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">密码</span>
            <input
              value={webdavPass}
              onChange={e => setWebdavPass(e.target.value)}
              type="password"
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
              disabled={webdavRunning}
            />
          </div>
          <div className="flex items-center gap-2 pt-1">
            <button
              onClick={toggleWebDAV}
              className={`inline-flex items-center gap-1.5 h-8 px-4 rounded-md text-xs transition-colors ${
                webdavRunning
                  ? 'bg-red-500 text-white hover:bg-red-600'
                  : 'bg-primary text-primary-foreground hover:opacity-90'
              }`}
            >
              {webdavRunning ? <WifiOff className="w-3.5 h-3.5" /> : <Wifi className="w-3.5 h-3.5" />}
              {webdavRunning ? '停止 WebDAV' : '启动 WebDAV'}
            </button>
          </div>
          {webdavRunning && webdavInfo && (
            <div className="mt-2 p-3 bg-green-50 dark:bg-green-900/20 rounded-lg text-xs text-green-700 dark:text-green-300 whitespace-pre-line">
              {webdavInfo}
            </div>
          )}
        </div>
      </div>

      {/* 生成日志开关 */}
      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Shield className="w-3.5 h-3.5" />
          生成日志
        </label>
        <p className="text-[11px] text-muted-foreground">AI 对话生成过程的日志写入文件。关闭后仅输出到控制台。</p>
        <button
          onClick={handleLoggingToggle}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
            loggingEnabled ? 'bg-primary' : 'bg-muted'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              loggingEnabled ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>

      <div className="mt-6 space-y-3">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <GitFork className="w-3.5 h-3.5" />
          {t('settings.gitConfig')}
        </label>
        <p className="text-[11px] text-muted-foreground">{t('settings.gitConfigDesc')}</p>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">{t('settings.nickname')}</span>
            <input
              value={gitName}
              onChange={e => setGitName(e.target.value)}
              placeholder="Goink"
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">{t('settings.email')}</span>
            <input
              value={gitEmail}
              onChange={e => setGitEmail(e.target.value)}
              placeholder="goink@local"
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="flex items-center justify-between gap-2 pt-1">
            <div className="flex items-center gap-2">
              {gitError && <span className="text-[11px] text-rose-500">{gitError}</span>}
            </div>
            <button
              onClick={handleSaveGit}
              disabled={gitSaving}
              className="inline-flex items-center gap-1.5 h-8 px-4 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
            >
              {gitSaving ? t('common.saving') : gitSaved ? t('common.saved') : t('common.save')}
            </button>
          </div>
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Languages className="w-3.5 h-3.5" />
          {t('settings.language')}
        </label>
        <div className="inline-flex items-center gap-1 rounded-lg bg-muted/60 p-0.5">
          <button
            onClick={() => i18n.changeLanguage('zh-CN')}
            className={`h-7 px-3 rounded-md text-xs transition-colors ${
              i18n.language.startsWith('zh') ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            中文
          </button>
          <button
            onClick={() => i18n.changeLanguage('en')}
            className={`h-7 px-3 rounded-md text-xs transition-colors ${
              i18n.language === 'en' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            English
          </button>
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground">{t('settings.maintenance')}</label>
        <p className="text-[11px] text-muted-foreground">{t('settings.rebuildIndexDesc')}</p>
        <div className="flex items-center gap-2">
          <select
            value={selectedID}
            onChange={e => setSelectedID(Number(e.target.value))}
            className="h-8 rounded-md border bg-background px-2 text-xs focus:outline-none"
          >
            {novels.map(n => (
              <option key={n.id} value={n.id}>{n.title}</option>
            ))}
          </select>
          <button
            onClick={handleRebuild}
            disabled={rebuilding || !selectedID}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${rebuilding ? 'animate-spin' : ''}`} />
            {rebuilding ? t('settings.rebuilding') : t('settings.rebuildIndex')}
          </button>
        </div>
      </div>

      {/* 数据备份与恢复 */}
      <div className="mt-6 space-y-3">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Archive className="w-3.5 h-3.5" />
          数据备份与恢复
        </label>
        <p className="text-[11px] text-muted-foreground">备份包含数据库、小说文件和技能。恢复会先自动备份当前数据。</p>

        <div className="space-y-2">
          <button
            onClick={handleBackup}
            disabled={backing}
            className="inline-flex items-center gap-1.5 h-8 px-4 rounded-md text-xs bg-primary text-primary-foreground hover:opacity-90 transition-colors disabled:opacity-50"
          >
            {backing ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Archive className="w-3.5 h-3.5" />}
            {backing ? '备份中...' : '立即备份'}
          </button>
          {backupPath && (
            <div className="p-2 bg-green-50 dark:bg-green-900/20 rounded-lg text-xs text-green-700 dark:text-green-300">
              备份成功：<span className="font-mono break-all">{backupPath}</span>
            </div>
          )}
          {backupError && (
            <div className="p-2 bg-red-50 dark:bg-red-900/20 rounded-lg text-xs text-red-600">
              {backupError}
            </div>
          )}
        </div>

        <div className="space-y-2">
          <input
            ref={restoreInputRef}
            type="file"
            accept=".zip"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) handleRestore(file)
              e.target.value = ''
            }}
          />
          <button
            onClick={() => restoreInputRef.current?.click()}
            disabled={restoring}
            className="inline-flex items-center gap-1.5 h-8 px-4 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
          >
            {restoring ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <RotateCcw className="w-3.5 h-3.5" />}
            {restoring ? '恢复中...' : '选择备份文件恢复'}
          </button>
          {restoreMsg && (
            <div className={`p-2 rounded-lg text-xs ${restoreMsg.includes('成功') ? 'bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300' : 'bg-red-50 dark:bg-red-900/20 text-red-600'}`}>
              {restoreMsg}
            </div>
          )}
        </div>
      </div>

    </div>
  )
}
