import { useCallback, useRef } from 'react'
import type { novel, chapter } from '@/hooks/useApp'
import NovelList from './NovelList'
import ChapterList from './ChapterList'
import CharacterList from '@/components/character/CharacterList'
import LocationList from '@/components/location/LocationList'
import SkillList from '@/components/skill/SkillList'
import SearchPanel from '@/components/search/SearchPanel'
import TimelineList from '@/components/timeline/TimelineList'
import ArcList from '@/components/storyarc/ArcList'
import ReaderList from '@/components/reader/ReaderList'
import PreferenceList from '@/components/preference/PreferenceList'
import StyleSampleList from '@/components/style/StyleSampleList'
import LoreList from '@/components/lore/LoreList'
import ItemList from '@/components/item/ItemList'
import StatsList from '@/components/stats/StatsList'
import type { SearchResult } from '@/components/search/SearchPanel'
import GitHistoryList from '@/components/git/GitHistoryList'
import type { git } from '@/lib/wailsjs/go/models'

interface Props {
  activePanel: string
  novels: novel.Novel[]
  novelId: number
  onSelectNovel: (n: novel.Novel) => void
  onSelectChapter: (ch: chapter.Chapter) => void
  onSelectGoink: () => void
  onSelectBookOutline: () => void
  onExportNovel: (novelId: number) => void
  target: { path: string; title: string } | null
  showCreate: boolean
  setShowCreate: (v: boolean) => void
  title: string
  setTitle: (v: string) => void
  description: string
  setDescription: (v: string) => void
  onCreateNovel: () => void
  activeSkillName: string | null
  onSelectSkill: (path: string, title: string, readOnly: boolean) => void
  onEditSkill: (path: string, title: string, readOnly: boolean) => void
  onNewSkill: (name: string) => void
  onSearchNavigateEntity: (panelId: string, entityId: number) => void
  onSearchNavigateChapter: (filePath: string, title: string, chapterNum: number, matchPos: number, matchLen: number) => void
  searchQuery: string
  searchResults: SearchResult[]
  onSearchChange: (query: string, results: SearchResult[]) => void
  onSelectGitFile: (file: git.FileDiff) => void
  onSelectStyleSample: (id: number) => void
  sidePanelWidth: number
  onWidthChange?: (w: number) => void
}

export default function SidePanel({
  activePanel,
  novels, novelId, onSelectNovel,
  onSelectChapter, onSelectGoink, onSelectBookOutline, onExportNovel, target,
  showCreate, setShowCreate, title, setTitle, description, setDescription,
  onCreateNovel,
  activeSkillName, onSelectSkill, onEditSkill, onNewSkill,
  onSearchNavigateEntity, onSearchNavigateChapter,
  searchQuery, searchResults, onSearchChange,
  onSelectGitFile,
  onSelectStyleSample,
  sidePanelWidth,
  onWidthChange,
}: Props) {
  const dragRef = useRef(false)
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (!onWidthChange) return
    e.preventDefault()
    dragRef.current = true
    const startX = e.clientX, startW = sidePanelWidth
    const onMove = (ev: MouseEvent) => { if (dragRef.current) onWidthChange(startW + (ev.clientX - startX)) }
    const onUp = () => { dragRef.current = false; document.removeEventListener('mousemove', onMove); document.removeEventListener('mouseup', onUp); document.body.style.cursor = ''; document.body.style.userSelect = '' }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }, [sidePanelWidth, onWidthChange])
  return (
    <aside className="side-panel shrink-0 flex flex-col bg-sidebar border-r relative" style={{ width: sidePanelWidth }}>
      {activePanel === 'search' ? (
        <SearchPanel
          novelId={novelId}
          query={searchQuery}
          results={searchResults}
          onResultsChange={onSearchChange}
          onNavigateEntity={onSearchNavigateEntity}
          onNavigateChapter={onSearchNavigateChapter}
        />
      ) : activePanel === 'skills' ? (
        <SkillList
          novelId={novelId}
          activeSkillName={activeSkillName}
          onSelectSkill={onSelectSkill}
          onEditSkill={onEditSkill}
          onNewSkill={onNewSkill}
        />
      ) : activePanel === 'novels' ? (
        <NovelList
          novels={novels}
          novelId={novelId}
          onSelectNovel={onSelectNovel}
          showCreate={showCreate}
          setShowCreate={setShowCreate}
          title={title}
          setTitle={setTitle}
          description={description}
          setDescription={setDescription}
          onCreateNovel={onCreateNovel}
        />
      ) : activePanel === 'chapters' ? (
        <ChapterList
          novelId={novelId}
          target={target}
          onSelectChapter={onSelectChapter}
          onSelectGoink={onSelectGoink}
          onSelectBookOutline={onSelectBookOutline}
          onExportNovel={() => onExportNovel(novelId)}
        />
      ) : activePanel === 'characters' ? (
        <CharacterList novelId={novelId} />
      ) : activePanel === 'locations' ? (
        <LocationList novelId={novelId} />
      ) : activePanel === 'storyarcs' ? (
        <ArcList novelId={novelId} />
      ) : activePanel === 'timeline' ? (
        <TimelineList novelId={novelId} />
      ) : activePanel === 'reader' ? (
        <ReaderList novelId={novelId} />
      ) : activePanel === 'preferences' ? (
        <PreferenceList novelId={novelId} />
      ) : activePanel === 'world' ? (
        <LoreList novelId={novelId} />
      ) : activePanel === 'items' ? (
        <ItemList novelId={novelId} />
      ) : activePanel === 'stats' ? (
        <StatsList novelId={novelId} />
      ) : activePanel === 'git' ? (
        <GitHistoryList
          novelId={novelId}
          onSelectFile={onSelectGitFile}
        />
      ) : activePanel === 'style-samples' ? (
        <StyleSampleList
          onSelectSample={onSelectStyleSample}
          novelId={novelId}
        />
      ) : (
        <div />
      )}
      {onWidthChange && <div className="resize-handle" onMouseDown={handleMouseDown} />}
    </aside>
  )
}
