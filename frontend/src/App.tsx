import { useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import './App.css'
import type { Analysis } from './types'
import Prism from 'prismjs'
import 'prismjs/components/prism-go'

const API_BASE = 'http://localhost:8080'

interface BrowserEntry {
  name: string
  isDir: boolean
  path: string
}

function App() {
  const [analysis, setAnalysis] = useState<Analysis | null>(null)
  const [selectedStepIndex, setSelectedStepIndex] = useState(0)
  const [fileContent, setFileContent] = useState<string>('')
  const [expandedGaps, setExpandedGaps] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isPickingFolder, setIsPickingFolder] = useState(false)
  const [browserPath, setBrowserPath] = useState<string>('')
  const [browserEntries, setBrowserEntries] = useState<BrowserEntry[]>([])

  // Initial load: get home dir for browser
  useEffect(() => {
    fetch(`${API_BASE}/api/home`)
      .then(res => res.text())
      .then(home => setBrowserPath(home))
      .catch(err => console.error('Failed to get home dir:', err))
  }, [])

  // Browser navigation
  useEffect(() => {
    if (isPickingFolder && browserPath) {
      fetch(`${API_BASE}/api/ls?path=${encodeURIComponent(browserPath)}`)
        .then(res => res.json())
        .then(data => setBrowserEntries(data))
        .catch(err => console.error('Failed to list dir:', err))
    }
  }, [isPickingFolder, browserPath])

  const analyzePath = (path: string) => {
    setLoading(true)
    setError(null)
    setIsPickingFolder(false)
    
    fetch(`${API_BASE}/api/analysis?path=${encodeURIComponent(path)}`)
      .then(res => {
        if (!res.ok) throw new Error(`Server returned ${res.status}: ${res.statusText}`)
        return res.json()
      })
      .then(data => {
        setAnalysis(data)
        setLoading(false)
        setSelectedStepIndex(0)
      })
      .catch(err => {
        console.error('Failed to fetch analysis:', err)
        setError(`Analysis failed. Ensure the directory contains valid Go files.`)
        setLoading(false)
      })
  }

  useEffect(() => {
    if (analysis && analysis.steps[selectedStepIndex]) {
      const step = analysis.steps[selectedStepIndex]
      fetch(`${API_BASE}/api/file?path=${encodeURIComponent(step.File)}`)
        .then(res => {
          if (!res.ok) throw new Error(`Failed to load file: ${res.statusText}`)
          return res.text()
        })
        .then(content => {
          setFileContent(content)
          setExpandedGaps(new Set())
        })
        .catch(err => console.error('Failed to fetch file:', err))
    }
  }, [analysis, selectedStepIndex])

  const toggleGap = (gapIndex: number) => {
    const newGaps = new Set(expandedGaps)
    if (newGaps.has(gapIndex)) {
      newGaps.delete(gapIndex)
    } else {
      newGaps.add(gapIndex)
    }
    setExpandedGaps(newGaps)
  }

  const renderCode = () => {
    if (!fileContent) return null
    const lines = fileContent.split('\n')
    const highlightedLines = lines.map(line => Prism.highlight(line, Prism.languages.go, 'go'))
    const ranges = selectedStep?.Slice?.Ranges || []
    const elements: ReactNode[] = []
    let currentLine = 1
    const sortedRanges = [...ranges].sort((a, b) => a.Start - b.Start)

    const addGap = (start: number, end: number, idx: number) => {
      const gapSize = end - start + 1
      const gapIdx = idx

      // If the gap is just 1 line, just show it subtly instead of a button
      if (gapSize === 1) {
        elements.push(
          <div key={`line-${start}`} className="code-line line-hidden">
            <div className="line-number">{start}</div>
            <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[start - 1] }} />
          </div>
        )
        return
      }

      if (expandedGaps.has(gapIdx)) {
        elements.push(
          <div 
            key={`gap-header-${gapIdx}`} 
            className="expand-button" 
            onClick={() => toggleGap(gapIdx)}
          >
            ▲ Hide {gapSize} implementation lines
          </div>
        )
        for (let i = start; i <= end; i++) {
          elements.push(
            <div key={`line-${i}`} className="code-line line-hidden">
              <div className="line-number">{i}</div>
              <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[i - 1] }} />
            </div>
          )
        }
      } else {
        elements.push(
          <div 
            key={`gap-${gapIdx}`} 
            className="expand-button" 
            onClick={() => toggleGap(gapIdx)}
          >
            ▼ Show {gapSize} hidden lines
          </div>
        )
      }
    }

    sortedRanges.forEach((range, idx) => {
      if (currentLine < range.Start) {
        addGap(currentLine, range.Start - 1, idx * 2)
        currentLine = range.Start
      }
      for (let i = range.Start; i <= range.End && i <= lines.length; i++) {
        elements.push(
          <div key={`line-${i}`} className="code-line line-important">
            <div className="line-number">{i}</div>
            <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[i - 1] }} />
          </div>
        )
      }
      currentLine = range.End + 1
    })

    if (currentLine <= lines.length) {
      addGap(currentLine, lines.length, -1)
    }
    return elements
  }

  // Welcome Screen
  if (!analysis && !loading && !isPickingFolder) {
    return (
      <div className="welcome-screen">
        <div className="welcome-card">
          <h1>Map any repository.</h1>
          <p>Codemap analyzes your codebase to identify core abstractions and optimal reading order, helping you understand complex systems in minutes.</p>
          <button className="primary-button" onClick={() => setIsPickingFolder(true)}>
            Select a Repository to Start
          </button>
          {error && <p className="error" style={{marginTop: '24px', color: '#cf222e', fontWeight: 500}}>{error}</p>}
        </div>
      </div>
    )
  }

  // Folder Picker Modal
  const renderFolderPicker = () => (
    <div className="browser-overlay" onClick={() => setIsPickingFolder(false)}>
      <div className="browser-modal" onClick={e => e.stopPropagation()}>
        <div className="browser-header">
          <div className="browser-header-top">
            <h2>Select Repository</h2>
            <button className="secondary-button" onClick={() => setIsPickingFolder(false)}>Cancel</button>
          </div>
          <div className="browser-path">{browserPath}</div>
        </div>
        <div className="browser-list">
          {browserEntries.map(entry => (
            <div 
              key={entry.path} 
              className="browser-item"
              onClick={() => entry.isDir ? setBrowserPath(entry.path) : null}
            >
              <span className="browser-item-icon">{entry.isDir ? '📁' : '📄'}</span>
              <span className="browser-item-name">{entry.name}</span>
            </div>
          ))}
        </div>
        <div className="browser-footer">
          <button className="primary-button" onClick={() => analyzePath(browserPath)}>
            Analyze This Folder
          </button>
        </div>
      </div>
    </div>
  )

  if (loading) return <div className="full-screen-message">Analyzing repository...</div>
  
  const selectedStep = analysis?.steps[selectedStepIndex]

  return (
    <div className="app-container">
      {isPickingFolder && renderFolderPicker()}
      
      <div className="pane sidebar">
        <div className="sidebar-header" style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
          <span>Codemap: {analysis?.repository}</span>
          <button 
            className="secondary-button" 
            style={{padding: '4px 8px', fontSize: '11px'}}
            onClick={() => setIsPickingFolder(true)}
          >
            Change
          </button>
        </div>
        <ul className="reading-list">
          {analysis?.steps.map((step, idx) => (
            <li 
              key={step.File} 
              className={`reading-item ${idx === selectedStepIndex ? 'active' : ''}`}
              onClick={() => setSelectedStepIndex(idx)}
            >
              <strong>{idx + 1}. {step.File.split('/').pop()}</strong>
              <span className="item-path">{step.File.replace(new RegExp(`.*${analysis.repository}/`), '')}</span>
            </li>
          ))}
        </ul>
      </div>

      <div className="pane code-viewer-container">
        <div className="code-viewer">
          {renderCode()}
        </div>
      </div>

      <div className="pane context-pane">
        <div className="context-header">File Context</div>
        {selectedStep && (
          <>
            <div className="context-section">
              <h3>Purpose</h3>
              <div className="context-content">{selectedStep.Purpose}</div>
            </div>
            <div className="context-section">
              <h3>Learning Objective</h3>
              <div className="context-content">{selectedStep.LearningObjective}</div>
            </div>
            {selectedStep.KeySymbols.length > 0 && (
              <div className="context-section">
                <h3>Key Symbols</h3>
                <div className="context-content">
                  {selectedStep.KeySymbols.map(sym => (
                    <span key={sym} className="symbol-tag">{sym}</span>
                  ))}
                </div>
              </div>
            )}
            {selectedStep.Unlocks.length > 0 && (
              <div className="context-section">
                <h3>Unlocks</h3>
                <div className="context-content">
                  {selectedStep.Unlocks.map(u => (
                    <div key={u} style={{margin: '4px 0', fontSize: '14px', color: 'var(--accent-color)', fontWeight: 500}}>
                      → {u}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

export default App
