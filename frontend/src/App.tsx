import { useState, useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import './App.css'
import type { Analysis, ChatMessage } from './types'
import Prism from 'prismjs'
import 'prismjs/components/prism-go'

const API_BASE = 'http://localhost:8080'

interface BrowserEntry {
  name: string
  isDir: boolean
  path: string
}

const DEFAULT_MODELS: Record<string, string> = {
  'Gemini': 'gemini-1.5-pro',
  'OpenAI': 'gpt-4o',
  'Anthropic': 'claude-3-5-sonnet-20241022',
  'Groq': 'llama-3.3-70b-versatile',
  'Ollama (Local)': 'llama3'
}

function App() {
  const [analysis, setAnalysis] = useState<Analysis | null>(null)
  const [selectedStepIndex, setSelectedStepIndex] = useState(0)
  const [fileContent, setFileContent] = useState<string>('')
  const [expandedGaps, setExpandedGaps] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  
  // File Browser State
  const [isPickingFolder, setIsPickingFolder] = useState(false)
  const [browserPath, setBrowserPath] = useState<string>('')
  const [browserEntries, setBrowserEntries] = useState<BrowserEntry[]>([])

  // Layout State
  const [leftWidth, setLeftWidth] = useState(300)
  const [rightWidth, setRightWidth] = useState(450)
  const isDraggingLeft = useRef(false)
  const isDraggingRight = useRef(false)

  // AI State
  const [provider, setProvider] = useState<string>(() => localStorage.getItem('ai_provider') || '')
  const [model, setModel] = useState<string>(() => localStorage.getItem('ai_model') || '')
  const [availableProviders, setAvailableProviders] = useState<string[]>(['Ollama (Local)'])
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const [aiContexts, setAiContexts] = useState<Record<string, {purpose: string, objective: string}>>({})
  const [chatHistories, setChatHistories] = useState<Record<string, ChatMessage[]>>({})
  const [chatInput, setChatInput] = useState('')
  const [generatingContext, setGeneratingContext] = useState(false)
  const [sendingChat, setSendingChat] = useState(false)

  const chatHistoryRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (provider) {
      localStorage.setItem('ai_provider', provider)
    }
  }, [provider])

  useEffect(() => {
    if (model) {
      localStorage.setItem('ai_model', model)
    }
  }, [model])

  useEffect(() => {
    if (!provider) {
      setAvailableModels([])
      return
    }
    
    setAvailableModels(['Loading models...'])
    
    fetch(`${API_BASE}/api/ai/models?provider=${encodeURIComponent(provider)}`)
      .then(res => res.json())
      .then((data: string[]) => {
        if (!data || data.length === 0) {
          setAvailableModels([])
          return
        }
        
        setAvailableModels(data)
        const savedModel = localStorage.getItem('ai_model')
        
        if (savedModel && data.includes(savedModel)) {
          setModel(savedModel)
        } else if (DEFAULT_MODELS[provider] && data.includes(DEFAULT_MODELS[provider])) {
          setModel(DEFAULT_MODELS[provider])
        } else {
          setModel(data[0])
        }
      })
      .catch(err => {
        console.error('Failed to get models:', err)
        setAvailableModels([])
      })
  }, [provider])

  useEffect(() => {
    // Fetch available providers based on .env keys
    fetch(`${API_BASE}/api/ai/providers`)
      .then(res => res.json())
      .then((data: string[]) => {
        setAvailableProviders(data)
        const savedProvider = localStorage.getItem('ai_provider')
        
        if (savedProvider && data.includes(savedProvider)) {
          setProvider(savedProvider)
        } else if (data.length > 0) {
          const bestDefault = data.find(p => p !== 'Ollama (Local)') || data[0]
          setProvider(bestDefault)
        }
      })
      .catch(err => console.error('Failed to get providers:', err))

    fetch(`${API_BASE}/api/home`)
      .then(res => res.text())
      .then(home => setBrowserPath(home))
      .catch(err => console.error('Failed to get home dir:', err))
  }, [])

  useEffect(() => {
    if (isPickingFolder && browserPath) {
      fetch(`${API_BASE}/api/ls?path=${encodeURIComponent(browserPath)}`)
        .then(res => res.json())
        .then(data => setBrowserEntries(data))
        .catch(err => console.error('Failed to list dir:', err))
    }
  }, [isPickingFolder, browserPath])

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDraggingLeft.current) {
        setLeftWidth(e.clientX)
      } else if (isDraggingRight.current) {
        setRightWidth(window.innerWidth - e.clientX)
      }
    }
    const handleMouseUp = () => {
      isDraggingLeft.current = false
      isDraggingRight.current = false
      document.body.style.cursor = 'default'
    }
    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [])

  const analyzePath = (path: string) => {
    setLoading(true)
    setError(null)
    setIsPickingFolder(false)
    setAiContexts({})
    setChatHistories({})
    
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

  useEffect(() => {
    if (chatHistoryRef.current) {
      chatHistoryRef.current.scrollTop = chatHistoryRef.current.scrollHeight
    }
  }, [chatHistories, selectedStepIndex, sendingChat])

  const toggleGap = (gapIndex: number) => {
    const newGaps = new Set(expandedGaps)
    if (newGaps.has(gapIndex)) {
      newGaps.delete(gapIndex)
    } else {
      newGaps.add(gapIndex)
    }
    setExpandedGaps(newGaps)
  }

  const selectedStep = analysis?.steps[selectedStepIndex]
  const currentFile = selectedStep?.File || ''

  const handleGenerateContext = async () => {
    if (!currentFile || !fileContent) return
    setGeneratingContext(true)
    try {
      const res = await fetch(`${API_BASE}/api/ai/generate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider,
          model,
          fileContent,
          filePath: currentFile
        })
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      setAiContexts(prev => ({ ...prev, [currentFile]: data }))
    } catch (e: any) {
      alert(`AI Generation Failed: ${e.message}`)
    } finally {
      setGeneratingContext(false)
    }
  }

  const handleSendChat = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!chatInput.trim() || !currentFile || !fileContent || sendingChat) return

    const prompt = chatInput.trim()
    setChatInput('')
    setSendingChat(true)

    const currentHistory = chatHistories[currentFile] || []
    const updatedHistory: ChatMessage[] = [...currentHistory, { role: 'user', content: prompt }]
    setChatHistories(prev => ({ ...prev, [currentFile]: updatedHistory }))

    try {
      const res = await fetch(`${API_BASE}/api/ai/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider,
          model,
          fileContent,
          filePath: currentFile,
          history: currentHistory,
          prompt
        })
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      
      setChatHistories(prev => ({
        ...prev,
        [currentFile]: [...updatedHistory, { role: 'assistant', content: data.response }]
      }))
    } catch (e: any) {
      setChatHistories(prev => ({
        ...prev,
        [currentFile]: [...updatedHistory, { role: 'assistant', content: `Error: ${e.message}` }]
      }))
    } finally {
      setSendingChat(false)
    }
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
          <div key={`gap-header-${gapIdx}`} className="expand-button" onClick={() => toggleGap(gapIdx)}>
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
          <div key={`gap-${gapIdx}`} className="expand-button" onClick={() => toggleGap(gapIdx)}>
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
  
  const aiCtx = aiContexts[currentFile]
  const currentHistory = chatHistories[currentFile] || []

  return (
    <div className="app-container" style={{ gridTemplateColumns: `${leftWidth}px 1fr ${rightWidth}px` }}>
      {isPickingFolder && renderFolderPicker()}
      
      <div className="pane sidebar" style={{ width: leftWidth }}>
        <div className="sidebar-header" style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
          <span style={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginRight: '10px'}}>
            Codemap: {analysis?.repository}
          </span>
          <button className="secondary-button" style={{padding: '4px 8px', fontSize: '11px', flexShrink: 0}} onClick={() => setIsPickingFolder(true)}>
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
              <span className="item-path">{step.File.replace(new RegExp(`.*${analysis?.repository}/`), '')}</span>
            </li>
          ))}
        </ul>
      </div>

      <div 
        className="resizer" 
        onMouseDown={() => { isDraggingLeft.current = true; document.body.style.cursor = 'col-resize' }} 
      />

      <div className="pane code-viewer-container" style={{ flex: 1 }}>
        <div className="code-viewer">
          {renderCode()}
        </div>
      </div>

      <div 
        className="resizer" 
        onMouseDown={() => { isDraggingRight.current = true; document.body.style.cursor = 'col-resize' }} 
      />

      <div className="pane context-pane" style={{ width: rightWidth }}>
        <div className="context-header" style={{flexWrap: 'wrap', gap: '8px'}}>
          <h2 style={{margin: 0, fontSize: '16px'}}>AI Assistant</h2>
          <div style={{display: 'flex', gap: '8px'}}>
            <select 
              className="model-select" 
              value={provider} 
              onChange={e => setProvider(e.target.value)}
            >
              {availableProviders.map(p => <option key={p} value={p}>{p}</option>)}
            </select>
            <select 
              className="model-select" 
              value={model} 
              onChange={e => setModel(e.target.value)}
              style={{width: '180px'}}
            >
              {availableModels.map(m => <option key={m} value={m}>{m}</option>)}
            </select>
          </div>
        </div>
        
        <div className="context-body">
          {/* Static Unlocks / Symbols */}
          {selectedStep && selectedStep.Unlocks.length > 0 && (
            <div className="context-section" style={{marginBottom: 0}}>
              <h3>Unlocks</h3>
              <div className="context-content">
                {selectedStep.Unlocks.map(u => (
                  <div key={u} style={{margin: '4px 0', fontSize: '13px', color: 'var(--accent-color)', fontWeight: 500}}>
                    → {u}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* AI Context Generation */}
          <div className="context-section" style={{marginBottom: 0}}>
            {aiCtx ? (
              <>
                <div style={{marginBottom: '16px'}}>
                  <h3>Purpose</h3>
                  <div className="context-content">{aiCtx.purpose}</div>
                </div>
                <div>
                  <h3>Learning Objective</h3>
                  <div className="context-content">{aiCtx.objective}</div>
                </div>
              </>
            ) : (
              <button 
                className="ai-generate-btn" 
                onClick={handleGenerateContext}
                disabled={generatingContext}
              >
                {generatingContext ? 'Analyzing File...' : 'Generate AI Context'}
              </button>
            )}
          </div>

          {/* Chat Interface */}
          <div className="chat-container">
            <div className="chat-history" ref={chatHistoryRef}>
              {currentHistory.length === 0 && (
                <div style={{color: 'var(--text-secondary)', textAlign: 'center', margin: 'auto', fontSize: '13px'}}>
                  Ask a question about this file.
                </div>
              )}
              {currentHistory.map((msg, i) => (
                <div key={i} className={`chat-message ${msg.role}`}>
                  {msg.content}
                </div>
              ))}
              {sendingChat && (
                <div className="chat-message assistant" style={{opacity: 0.7}}>
                  Thinking...
                </div>
              )}
            </div>
            <form className="chat-input-area" onSubmit={handleSendChat}>
              <input 
                type="text" 
                className="chat-input" 
                placeholder="Ask about this file..." 
                value={chatInput}
                onChange={e => setChatInput(e.target.value)}
                disabled={sendingChat}
              />
              <button type="submit" className="chat-submit" disabled={sendingChat || !chatInput.trim()}>
                Send
              </button>
            </form>
          </div>
        </div>
      </div>
    </div>
  )
}

export default App
