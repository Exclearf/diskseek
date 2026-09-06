import { useRef, useState, type FormEvent, type MouseEvent } from 'react'
import {
  Box,
  Button,
  CircularProgress,
  Container,
  Link,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'

type DatasetID = 'wiki' | 'bee'

type SearchResult = {
  external_id: string
  title: string
  preview: string
  source_url: string
  score: number
}

type SearchResponse = {
  results: SearchResult[]
}

const datasets: Record<DatasetID, { label: string; source: string; examples: string[] }> = {
  wiki: {
    label: 'Wikipedia',
    source: 'Simple English Wikipedia',
    examples: ['black holes', 'honey bee', 'computer science'],
  },
  bee: {
    label: 'Bee Movie',
    source: 'Script passages',
    examples: ['the hive', 'flying outside', 'honey'],
  },
}

export default function App() {
  const [dataset, setDataset] = useState<DatasetID>('wiki')
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const request = useRef<AbortController | null>(null)

  async function search(nextQuery: string, nextDataset = dataset) {
    const trimmedQuery = nextQuery.trim()
    if (!trimmedQuery) {
      setError('Enter something to search for.')
      return
    }

    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    setError('')
    setResults(null)

    try {
      const response = await fetch('/v1/search', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dataset: nextDataset, query: trimmedQuery }),
        signal: controller.signal,
      })
      if (!response.ok) {
        throw new Error('Search is unavailable. Please try again.')
      }
      const output = (await response.json()) as SearchResponse
      if (request.current === controller) {
        setResults(output.results)
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
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    void search(query)
  }

  function selectDataset(_: MouseEvent<HTMLElement>, value: DatasetID | null) {
    if (value === null || value === dataset) {
      return
    }
    request.current?.abort()
    request.current = null
    setDataset(value)
    setResults(null)
    setLoading(false)
    setError('')
  }

  const selected = datasets[dataset]
  const searched = loading || results !== null || error !== ''

  return (
    <Container
      maxWidth="md"
      component="main"
      sx={{ pt: searched ? { xs: 4, md: 6 } : { xs: 10, md: 18 }, pb: 6 }}
    >
      <Typography
        component="h1"
        sx={{ fontSize: searched ? 28 : 46, fontWeight: 600, letterSpacing: '-0.04em', mb: 3 }}
      >
        DiskSeek
      </Typography>

      <Box component="form" onSubmit={submit} sx={{ maxWidth: 720 }}>
        <ToggleButtonGroup
          exclusive
          value={dataset}
          onChange={selectDataset}
          aria-label="Dataset"
          size="small"
          sx={{ mb: 2 }}
        >
          {(Object.keys(datasets) as DatasetID[]).map((id) => (
            <ToggleButton
              key={id}
              value={id}
              sx={id === 'bee' ? { '&.Mui-selected': { bgcolor: '#fce8b2' } } : undefined}
            >
              {datasets[id].label}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>

        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
          <TextField
            fullWidth
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search this dataset…"
            slotProps={{ htmlInput: { 'aria-label': 'Search query' } }}
          />
          <Button type="submit" variant="contained" size="large" disabled={loading} sx={{ px: 4 }}>
            Search
          </Button>
        </Stack>

        <Typography color="text.secondary" sx={{ mt: 1.5 }}>
          {selected.source}
        </Typography>
        <Stack direction="row" spacing={1} useFlexGap sx={{ mt: 1, flexWrap: 'wrap' }}>
          <Typography color="text.secondary">Try:</Typography>
          {selected.examples.map((example) => (
            <Button
              key={example}
              variant="text"
              size="small"
              sx={{ minWidth: 0, p: 0 }}
              onClick={() => {
                setQuery(example)
                void search(example)
              }}
            >
              {example}
            </Button>
          ))}
        </Stack>
      </Box>

      <Box aria-live="polite" sx={{ maxWidth: 720, mt: 5 }}>
        {loading && <CircularProgress size={28} aria-label="Searching" />}
        {error && <Typography color="error">{error}</Typography>}
        {results?.length === 0 && <Typography>No results found.</Typography>}
        {results && results.length > 0 && (
          <>
            <Typography color="text.secondary" sx={{ mb: 3 }}>
              {results.length} {results.length === 1 ? 'result' : 'results'} · {selected.label}
            </Typography>
            <Stack component="ol" spacing={4} sx={{ p: 0, m: 0, listStyle: 'none' }}>
              {results.map((result) => (
                <Box component="li" key={result.external_id}>
                  <Typography component="h2" sx={{ fontSize: 21, lineHeight: 1.3 }}>
                    {result.source_url ? (
                      <Link href={result.source_url} target="_blank" rel="noreferrer">
                        {result.title}
                      </Link>
                    ) : (
                      result.title
                    )}
                  </Typography>
                  {result.source_url && (
                    <Typography color="text.secondary" sx={{ fontSize: 14, my: 0.5 }}>
                      {new URL(result.source_url).hostname}
                    </Typography>
                  )}
                  <Typography sx={{ lineHeight: 1.55 }}>{result.preview}</Typography>
                </Box>
              ))}
            </Stack>
          </>
        )}
      </Box>
    </Container>
  )
}
