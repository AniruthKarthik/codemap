import { useState, useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import './App.css'
import type { Analysis, ChatMessage } from './types'
import Prism from 'prismjs'
import 'prismjs/components/prism-go'
import ReactMarkdown from 'react-markdown'

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
    
    const visibleLines = new Set<number>()
    ranges.forEach(r => {
      for (let i = r.Start; i <= r.End; i++) {
        visibleLines.add(i)
      }
    })

    let inBlockComment = false
    let inSignature = false
    let openParens = 0

    lines.forEach((line, index) => {
      const lineNum = index + 1
      const trimmed = line.trim()
      
      if (trimmed.startsWith('/*') || trimmed.includes('/*')) {
        inBlockComment = true
      }
      
      const parenOpens = (line.match(/\(/g) || []).length
      const parenCloses = (line.match(/\)/g) || []).length
      openParens += (parenOpens - parenCloses)

      if (inSignature) {
        visibleLines.add(lineNum)
        if (trimmed.includes('{')) {
          inSignature = false
        } else if (openParens <= 0 && !trimmed.endsWith(',') && !trimmed.endsWith('(')) {
          inSignature = false
        }
      } else {
        if (
          inBlockComment || 
          trimmed.startsWith('//') || 
          trimmed.startsWith('package ') || 
          trimmed.startsWith('import ') ||
          trimmed.startsWith('func ') || 
          trimmed.startsWith('type ')
        ) {
          visibleLines.add(lineNum)
          
          if (trimmed.startsWith('func ') || trimmed.startsWith('type ') || trimmed.startsWith('import ')) {
            if (!trimmed.includes('{') && (trimmed.endsWith(',') || trimmed.endsWith('(') || openParens > 0)) {
              inSignature = true
            }
          }
        }
      }
      
      if (trimmed.includes('*/')) {
        inBlockComment = false
      }
    })

    const blocks: { type: 'visible' | 'hidden', start: number, end: number }[] = []
    let currentType: 'visible' | 'hidden' | null = null
    let currentStart = 1
    
    for (let i = 1; i <= lines.length; i++) {
      const type = visibleLines.has(i) ? 'visible' : 'hidden'
      if (currentType === null) {
        currentType = type
        currentStart = i
      } else if (currentType !== type) {
        blocks.push({ type: currentType, start: currentStart, end: i - 1 })
        currentType = type
        currentStart = i
      }
    }
    if (currentType !== null) {
      blocks.push({ type: currentType, start: currentStart, end: lines.length })
    }

    const highlights = selectedStep?.Slice?.Highlights || []
    const lineToHighlight = new Map<number, typeof highlights[0]>()
    highlights.forEach(h => {
      lineToHighlight.set(h.Start, h)
    })

    blocks.forEach((block, idx) => {
      if (block.type === 'visible') {
        for (let i = block.start; i <= block.end; i++) {
          elements.push(
            <div key={`line-${i}`} className="code-line line-important" style={{ position: 'relative' }}>
              <div className="line-number">{i}</div>
              <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[i - 1] }} />
              {lineToHighlight.has(i) && (
                <div className="highlight-reason">
                  {lineToHighlight.get(i)!.Reason}
                </div>
              )}
            </div>
          )
        }
      } else {
        const gapSize = block.end - block.start + 1
        if (gapSize <= 3) {
          for (let i = block.start; i <= block.end; i++) {
            elements.push(
              <div key={`line-${i}`} className="code-line line-hidden">
                <div className="line-number">{i}</div>
                <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[i - 1] }} />
              </div>
            )
          }
        } else {
          if (expandedGaps.has(idx)) {
            elements.push(
              <div key={`gap-header-${idx}`} className="expand-button" onClick={() => toggleGap(idx)}>
                ▲ Hide {gapSize} lines
              </div>
            )
            for (let i = block.start; i <= block.end; i++) {
              elements.push(
                <div key={`line-${i}`} className="code-line line-hidden">
                  <div className="line-number">{i}</div>
                  <div className="line-content" dangerouslySetInnerHTML={{ __html: highlightedLines[i - 1] }} />
                </div>
              )
            }
          } else {
            elements.push(
              <div key={`gap-${idx}`} className="expand-button" onClick={() => toggleGap(idx)}>
                ▼ Show {gapSize} hidden lines
              </div>
            )
          }
        }
      }
    })

    return elements
  }

  if (!analysis && !loading && !isPickingFolder) {
    return (
      <div className="welcome-screen">
        <div className="welcome-screen-bg">
           <div className="blob blob-1"></div>
           <div className="blob blob-2"></div>
           <div className="blob blob-3"></div>
        </div>
        <div className="welcome-content">
          <div className="welcome-text-section">
            <div className="badge">Codemap 1.0</div>
            <h1 className="gradient-text">Map any repository.</h1>
            <p>Codemap analyzes your codebase to identify core abstractions and optimal reading order, helping you understand complex systems in minutes.</p>
            <button className="primary-button large-button" onClick={() => setIsPickingFolder(true)}>
              Select a Repository to Start
            </button>
            {error && <p className="error" style={{marginTop: '24px', color: '#ff3b30', fontWeight: 500}}>{error}</p>}
          </div>
          
          <div className="welcome-features-grid">
            <div className="feature-card" style={{ animationDelay: '0.1s' }}>
              <div className="feature-icon feature-icon-blue">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"></path><polyline points="3.27 6.96 12 12.01 20.73 6.96"></polyline><line x1="12" y1="22.08" x2="12" y2="12"></line></svg>
              </div>
              <h3>Identify Abstractions</h3>
              <p>Instantly find the most important files and structures without reading everything.</p>
            </div>
            <div className="feature-card" style={{ animationDelay: '0.2s' }}>
              <div className="feature-icon feature-icon-purple">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="18" cy="18" r="3"></circle><circle cx="6" cy="6" r="3"></circle><path d="M13 6h3a2 2 0 0 1 2 2v7"></path><line x1="6" y1="9" x2="6" y2="21"></line></svg>
              </div>
              <h3>Reading Order</h3>
              <p>Learn exactly which files to read first, and trace dependencies step-by-step.</p>
            </div>
            <div className="feature-card" style={{ animationDelay: '0.3s' }}>
              <div className="feature-icon feature-icon-pink">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z"></path><path d="M5 3v4"></path><path d="M19 17v4"></path><path d="M3 5h4"></path><path d="M17 19h4"></path></svg>
              </div>
              <h3>AI Assistant Built-in</h3>
              <p>Ask questions about any file, generate contexts with local LLMs.</p>
            </div>
            <div className="feature-card" style={{ animationDelay: '0.4s' }}>
              <div className="feature-icon feature-icon-orange">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>
              </div>
              <h3>Blazing Fast</h3>
              <p>Analyzes massive codebases in seconds using an optimized Go parser.</p>
            </div>
          </div>
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
                <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px'}}>
                  <h3 style={{margin: 0}}>Purpose</h3>
                  <button 
                    className="secondary-button" 
                    style={{padding: '4px 8px', fontSize: '11px', marginTop: '-4px'}}
                    onClick={handleGenerateContext}
                    disabled={generatingContext}
                  >
                    {generatingContext ? 'Retrying...' : '↻ Regenerate'}
                  </button>
                </div>
                <div style={{marginBottom: '16px'}}>
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
                  {msg.role === 'assistant' ? (
                    <ReactMarkdown>{msg.content}</ReactMarkdown>
                  ) : (
                    msg.content
                  )}
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
