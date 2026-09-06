import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import BarChartOutlined from '@mui/icons-material/BarChartOutlined'
import CloseRounded from '@mui/icons-material/CloseRounded'
import DarkModeOutlined from '@mui/icons-material/DarkModeOutlined'
import GitHubIcon from '@mui/icons-material/GitHub'
import InfoOutlined from '@mui/icons-material/InfoOutlined'
import Language from '@mui/icons-material/Language'
import LightModeOutlined from '@mui/icons-material/LightModeOutlined'
import OpenInNewRounded from '@mui/icons-material/OpenInNewRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import { Button, CircularProgress, IconButton, InputBase, Link, NativeSelect, useColorScheme } from '@mui/material'
import './App.css'

type SearchResult = {
  external_id: string
  title: string
  preview: string
  source_url: string
  score: number
}
type SearchResponse = { results: SearchResult[]; search_ms: number }

const examples = ['black holes', 'honey bee', 'computer science']

function readSearchLocation() {
  const params = new URLSearchParams(window.location.hash.slice(1))
  const requestedLimit = Number(params.get('results'))
  return {
    query: (params.get('search') ?? '').trim(),
    limit: [5, 10, 20].includes(requestedLimit) ? requestedLimit : 5,
  }
}

function pushSearchLocation(query: string, limit: number) {
  const url = new URL(window.location.href)
  url.search = ''
  url.hash = ''
  if (query) {
    const params = new URLSearchParams({ search: query })
    if (limit !== 5) params.set('results', String(limit))
    url.hash = params.toString()
  }
  if (url.href !== window.location.href) window.history.pushState(null, '', url)
}

export default function App() {
  const [limit, setLimit] = useState(() => readSearchLocation().limit)
  const [query, setQuery] = useState(() => readSearchLocation().query)
  const [submittedQuery, setSubmittedQuery] = useState('')
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [searchMS, setSearchMS] = useState<number | null>(null)
  const [loading, setLoading] = useState(() => Boolean(readSearchLocation().query))
  const [error, setError] = useState('')
  const request = useRef<AbortController | null>(null)
  const input = useRef<HTMLInputElement | null>(null)
  const { mode, systemMode, setMode } = useColorScheme()
  const dark = (mode === 'system' ? systemMode : mode) === 'dark'

  const resetSearch = useCallback(() => {
    request.current?.abort()
    request.current = null
    setResults(null)
    setSearchMS(null)
    setLoading(false)
    setError('')
    setSubmittedQuery('')
  }, [])

  const search = useCallback(async (nextQuery: string, nextLimit: number, updateHistory = true) => {
    const trimmedQuery = nextQuery.trim()
    if (!trimmedQuery) return
    if (updateHistory) pushSearchLocation(trimmedQuery, nextLimit)
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setSubmittedQuery(trimmedQuery)
    setLoading(true)
    setError('')
    setResults(null)
    setSearchMS(null)
    try {
      const response = await fetch('/v1/search', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: trimmedQuery, limit: nextLimit }),
        signal: controller.signal,
      })
      if (!response.ok) throw new Error('Search is unavailable. Please try again.')
      const output = (await response.json()) as SearchResponse
      if (request.current === controller) {
        setResults(output.results)
        setSearchMS(output.search_ms)
      }
    } catch (cause) {
      if (request.current === controller && !(cause instanceof DOMException && cause.name === 'AbortError')) {
        setError(cause instanceof Error ? cause.message : 'Search is unavailable. Please try again.')
      }
    } finally {
      if (request.current === controller) {
        request.current = null
        setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    function restoreLocation() {
      const location = readSearchLocation()
      setQuery(location.query)
      setLimit(location.limit)
      if (location.query) void search(location.query, location.limit, false)
      else resetSearch()
    }

    restoreLocation()
    window.addEventListener('popstate', restoreLocation)
    return () => {
      window.removeEventListener('popstate', restoreLocation)
      request.current?.abort()
      request.current = null
    }
  }, [resetSearch, search])

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!query.trim()) {
      input.current?.focus()
      return
    }
    void search(query, limit)
  }

  function changeQuery(nextQuery: string) {
    if (nextQuery.trim()) {
      setQuery(nextQuery)
      return
    }
    setQuery('')
    resetSearch()
    pushSearchLocation('', limit)
  }

  const searched = loading || results !== null || error !== ''
  const resultSummary = results === null ? '' : [
    query.trim() !== submittedQuery ? `Results for “${submittedQuery}”` : '',
    results.length < limit ? `${results.length} ${results.length === 1 ? 'result' : 'results'}` : '',
  ].filter(Boolean).join(' · ')

  return (
    <div className={`app ${searched ? 'app--results' : 'app--home'}`}>
      <a className="skip-link" href="#main">Skip to content</a>
      <header className="topbar">
        <Button
          className="theme-toggle"
          color="inherit"
          onClick={() => setMode(dark ? 'light' : 'dark')}
          aria-label={`Switch to ${dark ? 'light' : 'dark'} theme`}
          startIcon={dark ? <LightModeOutlined fontSize="small" /> : <DarkModeOutlined fontSize="small" />}
        >
          {dark ? 'Light' : 'Dark'} theme
        </Button>
      </header>

      <main id="main" className="main" tabIndex={-1}>
        <section className="search-section" aria-label="Search">
          <h1 className="wordmark">
            <button type="button" aria-label="DiskSeek home" onClick={() => {
              resetSearch()
              setQuery('')
              setLimit(5)
              pushSearchLocation('', 5)
              input.current?.focus()
            }}>
              Disk<span>Seek</span>
            </button>
          </h1>
          <form className="search-form" onSubmit={submit} role="search">
            <div className="search-field">
              <InputBase
                className="query-input"
                inputRef={input}
                fullWidth
                value={query}
                onChange={(event) => changeQuery(event.target.value)}
                placeholder="Search articles"
                inputProps={{ 'aria-label': 'Search query', autoComplete: 'off' }}
              />
              {query && (
                <IconButton className="clear-query" size="small" aria-label="Clear search" onClick={() => {
                  changeQuery('')
                  input.current?.focus()
                }}>
                  <CloseRounded fontSize="small" />
                </IconButton>
              )}
              <IconButton className="submit-search" type="submit" aria-label="Search" disabled={loading}>
                {loading ? <CircularProgress size={20} color="inherit" /> : <SearchRounded fontSize="small" />}
              </IconButton>
            </div>
          </form>
          {!searched && (
            <div className="examples">
              {examples.map((example) => (
                <Button key={example} size="small" onClick={() => {
                  setQuery(example)
                  void search(example, limit)
                }}>{example}</Button>
              ))}
            </div>
          )}
        </section>

        <section className="results" aria-label="Search results" aria-live="polite" aria-busy={loading}>
          {searched && (
            <div className="result-meta">
              <NativeSelect
                className="result-limit"
                disableUnderline
                inputProps={{ 'aria-label': 'Number of results' }}
                value={limit}
                onChange={(event) => {
                  const nextLimit = Number(event.target.value)
                  setLimit(nextLimit)
                  if (query.trim()) void search(query, nextLimit)
                }}
              >
                {[5, 10, 20].map((count) => <option key={count} value={count}>Top {count}</option>)}
              </NativeSelect>
              {resultSummary && <span>{resultSummary}</span>}
              {searchMS !== null && <span className="result-time">{searchMS.toFixed(2)} ms</span>}
            </div>
          )}
          {loading && <p className="result-summary">Searching Wikipedia…</p>}
          {error && <div className="message"><h2>Let’s try that again</h2><p>{error}</p></div>}
          {results?.length === 0 && (
            <div className="message">
              <h2>No results for “{submittedQuery}”</h2>
              <p>Try a different word or a more general search.</p>
            </div>
          )}
          {results && results.length > 0 && (
            <ol className="result-list">
              {results.map((result) => (
                <li className="result" key={result.external_id}>
                  <h2>
                    {result.source_url ? (
                      <Link href={result.source_url} target="_blank" rel="noreferrer" underline="hover">
                        {result.title}<OpenInNewRounded aria-hidden="true" />
                      </Link>
                    ) : result.title}
                  </h2>
                  <p className="result-preview">{result.preview}</p>
                </li>
              ))}
            </ol>
          )}
        </section>
      </main>
      <footer className="footer">
        <nav aria-label="Project links">
          <Link className="footer-link" href="https://github.com/Exclearf/diskseek" color="inherit" underline="hover" target="_blank" rel="noreferrer">
            <GitHubIcon aria-hidden="true" />
            GitHub
          </Link>
          <Link className="footer-link" href="https://github.com/Exclearf/diskseek/blob/main/benchmarks/README.md" color="inherit" underline="hover" target="_blank" rel="noreferrer">
            <BarChartOutlined aria-hidden="true" />
            Benchmarks
          </Link>
          <Link className="footer-link" href="https://simple.wikipedia.org" color="inherit" underline="hover" target="_blank" rel="noreferrer">
            <Language aria-hidden="true" />
            Wikipedia excerpts
          </Link>
          <Link className="footer-link" href="https://creativecommons.org/licenses/by-sa/4.0/" color="inherit" underline="hover" target="_blank" rel="noreferrer">
            <InfoOutlined aria-hidden="true" />
            CC BY-SA 4.0
          </Link>
        </nav>
      </footer>
    </div>
  )
}
