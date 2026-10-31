import { useEffect, useState } from 'react'
import { ArrowLeft, ArrowRight, Bell, BriefcaseBusiness, Check, ChevronDown, CircleHelp, FileUser, LayoutDashboard, Plus, Search, Settings2, Sparkles, Trash2, UserRound, X, Pencil, Target } from 'lucide-react'
import { importLegacyProfiles } from './profileImport'
import { fromISODate, normalizeProfileDates, toISODate } from './profileDates'

const STORAGE_KEY = 'magnas-hirepilot-v1'
const legacyKey = value => {
  let hash = 14695981039346656037n
  for (const byte of new TextEncoder().encode(value)) { hash ^= BigInt(byte); hash = BigInt.asUintN(64, hash * 1099511628211n) }
  return `legacy-${hash.toString(16).padStart(16, '0')}`
}
const freshProfile = () => ({
  personal: { name: '', email: '', phone: '', location: '', headline: '', summary: '' },
  skills: { programming: [], technologies: [], tools: [] }, education: [], experience: [], languages: [], certificates: [], awards: [],
  searches: [],
})
const starter = { ...freshProfile(), personal: { name: 'Sangam', email: '', phone: '', location: 'Bengaluru, India', headline: 'Software Engineer', summary: '' }, skills: { programming: ['Go', 'TypeScript'], technologies: ['PostgreSQL', 'Redis'], tools: ['Docker', 'Git'] } }
const loadLegacyProfiles = () => {
  try {
    const list = JSON.parse(localStorage.getItem(`${STORAGE_KEY}:profiles`))
    if (Array.isArray(list)) {
      const normalized = list.map(item => ({ ...item, legacyImportId: String(item.legacyImportId || item.id || legacyKey(JSON.stringify(item))) }))
      if (normalized.some((item, index) => item.legacyImportId !== list[index]?.legacyImportId)) {
        try { localStorage.setItem(`${STORAGE_KEY}:profiles`, JSON.stringify(normalized)) } catch { /* Deterministic keys still make retry safe if storage is read-only. */ }
      }
      return normalized
    }
    const legacy = JSON.parse(localStorage.getItem(STORAGE_KEY))
    if (legacy && (legacy.personal?.name || legacy.searches?.length || legacy.education?.length || legacy.experience?.length))
      return [{ id: id(), legacyImportId: 'legacy-single-profile', name: legacy.personal?.headline || legacy.personal?.name || 'My profile', ...legacy }]
  } catch { /* Keep malformed legacy storage untouched for recovery. */ }
  return []
}
const saveLegacyProfiles = profiles => {
  try {
    if (profiles.length) localStorage.setItem(`${STORAGE_KEY}:profiles`, JSON.stringify(profiles))
    else localStorage.removeItem(`${STORAGE_KEY}:profiles`)
    if (!profiles.length) {
      localStorage.removeItem(STORAGE_KEY)
      localStorage.removeItem(`${STORAGE_KEY}:active`)
    }
  } catch { /* Retain the original browser copy if storage is unavailable. */ }
}
const toPayload = p => ({ name: p.name || p.personal?.headline || p.personal?.name || 'My profile', data: normalizeProfileDates(p), ...(p.legacyImportId ? { legacy_import_id: p.legacyImportId } : {}) })
const fromPayload = p => ({ ...p.data, id: p.id, name: p.name })
const api = async (url, options = {}) => {
  const response = await fetch(url, { credentials: 'include', ...options, headers: { ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...(options.headers || {}) } })
  const body = response.status === 204 ? null : await response.json().catch(() => null)
  if (!response.ok) throw new Error(body?.error || 'Request failed.')
  return body
}
const id = () => crypto.randomUUID()
const blankSearch = () => ({ id: id(), name: '', titles: [''], technologies: '', region: '', workMode: [], salary: '', currency: 'INR', period: 'month', hours: 0, days: 0, months: 0 })

function useRoute() {
  const [path, setPath] = useState(location.pathname)
  useEffect(() => { const f = () => setPath(location.pathname); addEventListener('popstate', f); return () => removeEventListener('popstate', f) }, [])
  const go = (to) => { history.pushState({}, '', to); setPath(to); window.scrollTo(0, 0) }
  return [path, go]
}

function App() {
  const [profiles, setProfiles] = useState([])
  const [activeId, setActiveId] = useState('')
  const [profile, setProfile] = useState(freshProfile)
  const [path, go] = useRoute()
  const [auth, setAuth] = useState({ loading: true, user: null })
  const [authError, setAuthError] = useState('')
  const [authBusy, setAuthBusy] = useState(false)
  useEffect(() => { fetch('/api/v1/auth/me', { credentials: 'include' }).then(r => r.ok ? r.json() : null).then(user => setAuth({ loading: false, user })).catch(() => setAuth({ loading: false, user: null })) }, [])
  useEffect(() => { if (!auth.loading && !auth.user && !['/login', '/register'].includes(path)) go(`/login?next=${encodeURIComponent(path + location.search)}`); if (!auth.loading && auth.user && ['/login', '/register'].includes(path)) go('/profiles') }, [auth.loading, auth.user, path])
  const [saved, setSaved] = useState(false)
  const [profileError, setProfileError] = useState('')
  const [profileBusy, setProfileBusy] = useState(false)
  const [importError, setImportError] = useState('')
  const refreshProfiles = async () => {
    const rows = await api('/api/v1/profiles')
    const converted = rows.map(fromPayload)
    setProfiles(converted)
    const active = rows.find(row => row.active)
    const chosen = converted.find(p => p.id === active?.id) || converted[0]
    setActiveId(chosen?.id || '')
    setProfile(chosen || freshProfile())
    return converted
  }
  useEffect(() => {
    if (!auth.user || auth.loading) return
    let alive = true
    setImportError('')
    ;(async () => {
      try {
        const current = await api('/api/v1/profiles')
        if (!alive) return
        const legacy = loadLegacyProfiles()
        if (legacy.length) {
          const result = await importLegacyProfiles(legacy, current, (url, options) => api(url, options))
          if (!result.complete && alive) {
            saveLegacyProfiles(result.remaining)
            setImportError(`${result.remaining.length} saved profile${result.remaining.length === 1 ? '' : 's'} could not be imported and remain saved in this browser. ${result.error?.message || 'Check your connection and reload to retry.'}`)
          }
          if (result.complete) {
            localStorage.removeItem(STORAGE_KEY)
            localStorage.removeItem(`${STORAGE_KEY}:profiles`)
            localStorage.removeItem(`${STORAGE_KEY}:active`)
          }
        }
        await refreshProfiles()
      } catch (error) { if (alive) setImportError(`Could not load saved profiles. Your browser copy is preserved. ${error.message || 'Check your connection and reload to retry.'}`) }
    })()
    return () => { alive = false }
  }, [auth.user?.id, auth.loading])
  useEffect(() => { if (!auth.user || !activeId || path.startsWith('/profiles/') || !profiles.some(p => p.id === activeId)) return; setProfiles(items => items.map(item => item.id === activeId ? { ...item, ...profile } : item)) }, [profile, activeId, auth.user?.username, path])
  useEffect(() => { setSaved(true); const t = setTimeout(() => setSaved(false), 1600); return () => clearTimeout(t) }, [profile])
  const route = path.split('/').filter(Boolean)
  if (auth.loading) return <AuthShell><p className="auth-loading">Loading your account…</p></AuthShell>
  if (!auth.user && !['/login', '/register'].includes(path)) return null
  if (auth.user && ['/login', '/register'].includes(path)) return null
  if (!auth.user) return <AuthPage mode={path === '/register' ? 'register' : 'login'} busy={authBusy} error={authError} onSubmit={async (username, password) => { setAuthBusy(true); setAuthError(''); try { const response = await fetch(`/api/v1/auth/${path === '/register' ? 'register' : 'login'}`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) }); const data = await response.json(); if (!response.ok) throw new Error(data.error || 'Unable to authenticate.'); setAuth({ loading: false, user: data }); go(new URLSearchParams(location.search).get('next') || '/profiles') } catch (error) { setAuthError(error.message) } finally { setAuthBusy(false) } }} go={go}/>
  const activeSearch = route[0] === 'search' ? profile.searches.find(s => s.id === route[1]) : null
  const editorRoute = path === '/profiles/create' || /^\/profiles\/[^/]+\/edit$/.test(path)
  const nav = [{ label: 'Dashboard', path: '/jobs', icon: LayoutDashboard }, { label: 'My Profiles', path: '/profiles', icon: UserRound }, { label: 'Discover', path: '/discover', icon: Search }, { label: 'Match', path: '/match', icon: Target }]
  const commitProfile = async savedProfile => {
    setProfileBusy(true); setProfileError('')
    try {
      const existing = profiles.some(p => p.id === savedProfile.id)
      const result = await api(existing ? `/api/v1/profiles/${savedProfile.id}` : '/api/v1/profiles', { method: existing ? 'PUT' : 'POST', body: JSON.stringify(toPayload(savedProfile)) })
      const persisted = fromPayload(result)
      if (!existing && !activeId) await api(`/api/v1/profiles/${persisted.id}/activate`, { method: 'PUT' })
      await refreshProfiles(); go('/profiles')
    } catch (error) { setProfileError(error.message) } finally { setProfileBusy(false) }
  }
  const selectProfile = async p => { try { await api(`/api/v1/profiles/${p.id}/activate`, { method: 'PUT' }); await refreshProfiles() } catch (error) { setProfileError(error.message) } }
  const deleteProfile = async p => { if (!confirm(`Delete “${p.name || 'profile'}”? This cannot be undone.`)) return; try { await api(`/api/v1/profiles/${p.id}`, { method: 'DELETE' }); await refreshProfiles() } catch (error) { setProfileError(error.message) } }
  return <div className="app-shell">
    <aside className="sidebar">
      <a className="brand" href="/" onClick={e => { e.preventDefault(); go('/') }}><span className="brand-mark"><Sparkles size={17}/></span><span>magnas<span className="brand-light">hire</span></span></a>
      <div className="workspace"><div className="workspace-avatar">M</div><div><strong>Magnas Hiring Pilot</strong><small>Personal workspace</small></div><ChevronDown size={15}/></div>
      <p className="nav-label">WORKSPACE</p>
      <nav>{nav.map(({ label, path: target, icon: Icon }) => <button key={label} className={`nav-item ${(path === target || (target === '/profiles' && (path === '/profiles' || editorRoute)) || (target === '/discover' && (path.startsWith('/search') || path === '/discover')) || (target === '/match' && path === '/match')) ? 'active' : ''}`} onClick={() => go(target)}><Icon size={17}/>{label}</button>)}</nav>
      <div className="active-profile-side"><span>ACTIVE PROFILE</span><strong>{profiles.find(p=>p.id===activeId)?.name || profiles.find(p=>p.id===activeId)?.personal?.headline || profiles.find(p=>p.id===activeId)?.personal?.name || 'None selected'}</strong></div>
      <div className="sidebar-bottom"><div className="help-card"><CircleHelp size={18}/><strong>Need a hand?</strong><p>Find answers and tips to get the most out of Magnus.</p><button>Visit help center <ArrowRight size={14}/></button></div><div className="user-card"><div className="profile-avatar">{auth.user.username.slice(0, 1).toUpperCase()}</div><span><strong>{auth.user.username}</strong><small>Signed in</small></span><button className="logout-button" onClick={async () => { await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' }); setAuth({ loading: false, user: null }); go('/login') }}>Log out</button></div></div>
    </aside>
    <main className="main-area">{(profileError || importError) && <div className={`app-toast ${profileError ? 'error' : 'notice'}`} role="alert"><CircleHelp size={16}/><span>{profileError || importError}</span><button type="button" aria-label="Dismiss message" onClick={()=>{setProfileError('');setImportError('')}}><X size={15}/></button></div>}<header className="topbar"><div className="breadcrumb">Workspace <span>/</span> <strong>{path.startsWith('/search') ? (activeSearch?.name || 'New search') : path === '/jobs' ? 'Dashboard' : path === '/profiles' ? 'My Profiles' : editorRoute ? 'Profile editor' : path.slice(1)}</strong></div><div className="top-actions"><button className="icon-button" aria-label="Help"><CircleHelp size={18}/></button><button className="icon-button notification" aria-label="Notifications"><Bell size={18}/></button><div className="profile-avatar small">{(profile.personal.name || auth.user.username || 'S').slice(0, 1).toUpperCase()}</div></div></header>
      {path === '/jobs' ? <Dashboard searches={profile.searches} go={go}/> : path === '/profiles' ? <ProfilesPage profiles={profiles} activeId={activeId} onSelect={selectProfile} onDelete={deleteProfile} go={go}/> : path === '/profiles/create' || /^\/profiles\/[^/]+\/edit$/.test(path) ? path.endsWith('/edit') && !profiles.some(p=>p.id===route[1]) ? <ProfilesPage profiles={profiles} activeId={activeId} onSelect={selectProfile} onDelete={deleteProfile} go={go}/> : <ProfilePage key={`${path}:${route[1] || 'new'}`} profile={path.endsWith('/edit') ? profiles.find(p=>p.id===route[1]) : freshProfile()} go={go} saved={saved} editorTitle={path.endsWith('/edit') ? 'Edit Profile' : 'Create Profile'} profileId={path.endsWith('/edit') ? route[1] : null} onSave={commitProfile} busy={profileBusy} error={profileError}/> : route[0] === 'search' ? <SearchEditor search={activeSearch || blankSearch()} isNew={!activeSearch} onSave={(s, isNew) => { setProfile(p => ({ ...p, searches: isNew ? [...p.searches, s] : p.searches.map(old => old.id === s.id ? s : old) })); go('/profiles') }} onCancel={() => go('/profiles')}/> : path === '/discover' || path === '/match' ? <Dashboard searches={profile.searches} go={go}/> : <ProfilesPage profiles={profiles} activeId={activeId} onSelect={selectProfile} onDelete={deleteProfile} go={go}/>}
    </main>
  </div>
}

function PageTitle({ eyebrow, title, description, right }) { return <div className="page-title"><div><div className="eyebrow">{eyebrow}</div><h1>{title}</h1><p>{description}</p></div>{right}</div> }
function Section({ icon: Icon, title, description, children, action, className = '' }) { return <section className={`section-card ${className}`}><div className="section-heading"><div className="section-icon"><Icon size={17}/></div><div className="section-heading-copy"><h2>{title}</h2>{description && <p>{description}</p>}</div>{action}</div><div className="section-content">{children}</div></section> }
function Field({ label, value, onChange, placeholder = '', type = 'text', required = false, hint, ...props }) { const profileDate = type === 'month'; const dateValue = profileDate ? toISODate(value) : ''; return <label className="field"><span>{label}{required && <i> *</i>}</span>{profileDate ? <span className="date-input-controls"><input type="text" inputMode="numeric" value={value ?? ''} onChange={e => onChange(e.target.value)} placeholder="dd/mm/yyyy" aria-label={`${label} (dd/mm/yyyy)`} {...props}/><input className="date-picker" type="date" value={dateValue} aria-label={`Choose ${label}`} onChange={e => onChange(fromISODate(e.target.value))}/></span> : <input type={type} value={value ?? ''} onChange={e => onChange(e.target.value)} placeholder={placeholder} required={required} {...props}/ >}{hint && <small>{hint}</small>}</label> }
function TextArea({ label, value, onChange, placeholder, rows = 4 }) { return <label className="field"><span>{label}</span><textarea rows={rows} value={value || ''} onChange={e => onChange(e.target.value)} placeholder={placeholder}/></label> }
function AddButton({ children, onClick, subtle = false }) { return <button type="button" className={`add-button ${subtle ? 'subtle' : ''}`} onClick={onClick}><Plus size={15}/>{children}</button> }
function RemoveButton({ onClick, label = 'Remove entry' }) { return <button type="button" className="remove-button" onClick={onClick} aria-label={label}><Trash2 size={15}/><span>Remove</span></button> }
function Empty({ title, description, action }) { return <div className="empty-state"><div className="empty-icon"><Sparkles size={21}/></div><strong>{title}</strong><p>{description}</p>{action}</div> }

function ProfilePage({ profile: initialProfile, go, saved, editorTitle = 'Create Profile', profileId, onSave, busy, error }) {
  const [profile, setProfile] = useState(() => ({ ...freshProfile(), ...initialProfile, personal: { ...freshProfile().personal, ...(initialProfile?.personal || {}) } }))
  const patch = (key, value) => setProfile(p => ({ ...p, [key]: value }))
  const personal = profile.personal || freshProfile().personal
  const setPersonal = (key, value) => patch('personal', { ...personal, [key]: value })
  const fieldRows = (key, blank) => profile[key] || []
  const addRow = (key, blank) => patch(key, [...fieldRows(key).map(x => ({ ...x })), { id: id(), ...blank }])
  const updateRow = (key, index, values) => patch(key, fieldRows(key).map((x, i) => i === index ? { ...x, ...values } : x))
  const removeRow = (key, index) => patch(key, fieldRows(key).filter((_, i) => i !== index))
  const skills = profile.skills || { programming: [], technologies: [], tools: [] }
  const updateSkill = (category, value) => patch('skills', { ...skills, [category]: value.split(',').map(x => x.trim()).filter(Boolean) })
  const searchSummary = (s) => [s.titles?.filter(Boolean).join(', '), s.technologies, s.region, s.workMode?.join(', '), s.salary && `${s.currency || 'INR'} ${s.salary}/${s.period || 'month'}`, (s.hours || s.days || s.months) && `Posted ${[s.months && `${s.months} mo`, s.days && `${s.days} d`, s.hours && `${s.hours} h`].filter(Boolean).join(' ')}`].filter(Boolean)
  return <div className="page-wrap profile-page">
    <button className="back-link" onClick={() => go('/profiles')}><ArrowLeft size={16}/> Back to My Profiles</button>
    <PageTitle eyebrow="YOUR PROFESSIONAL IDENTITY" title={editorTitle} description="A strong profile helps us find the right opportunities for you." right={<div className="save-indicator"><span className={saved ? 'saved-dot' : ''}></span>{saved ? 'Saved' : 'All changes saved'}</div>}/>
    <div className="form-grid profile-name-field"><Field label="Profile name" value={profile.name || ''} onChange={v=>patch('name',v)} placeholder="e.g. Backend Engineer" required/></div>
    <Section icon={UserRound} title="Personal information" description="The essentials that help employers get to know you."><div className="form-grid"><Field label="Full name" value={personal.name} onChange={v => setPersonal('name', v)} placeholder="e.g. Sangam Patel" required/><Field label="Email address" type="email" value={personal.email} onChange={v => setPersonal('email', v)} placeholder="you@example.com"/><Field label="Phone number" type="tel" value={personal.phone} onChange={v => setPersonal('phone', v)} placeholder="+91 98765 43210"/><Field label="Current location" value={personal.location} onChange={v => setPersonal('location', v)} placeholder="City, Country"/><div className="full-width"><Field label="Professional headline" value={personal.headline} onChange={v => setPersonal('headline', v)} placeholder="e.g. Senior Software Engineer"/></div><div className="full-width"><TextArea label="Professional summary" value={personal.summary} onChange={v => setPersonal('summary', v)} placeholder="Share a little about your experience, strengths, and what you're looking for." rows={4}/></div></div></Section>
    <Section icon={Sparkles} title="Skills" description="Keep your strongest skills organized by type."><div className="skills-grid">{[['programming','Programming languages'],['technologies','Technologies'],['tools','Tools']].map(([k,label])=><label className="field" key={k}><span>{label}</span><input value={(skills[k] || []).join(', ')} onChange={e => updateSkill(k,e.target.value)} placeholder="Add skills separated by commas"/><small>Separate each skill with a comma.</small><div className="skill-chips">{(skills[k] || []).map(s => <span key={s}>{s}<button type="button" aria-label={`Remove ${s}`} onClick={() => patch('skills',{...skills,[k]:skills[k].filter(x=>x!==s)})}><X size={12}/></button></span>)}</div></label>)}</div></Section>
    <Section icon={BriefcaseBusiness} title="Education" description="Add your degrees, courses, or other education." action={<AddButton onClick={() => addRow('education',{ institution:'', degree:'', field:'', startDate:'', endDate:'' })}>Add education</AddButton>}>
      {fieldRows('education').length ? <div className="entry-list">{fieldRows('education').map((entry,i)=><div className="entry-card" key={entry.id || i}><div className="entry-top"><strong>Education {i+1}</strong><RemoveButton onClick={() => removeRow('education',i)}/></div><div className="form-grid"><Field label="Institution" value={entry.institution} onChange={v=>updateRow('education',i,{institution:v})} placeholder="University or school" required/><Field label="Degree" value={entry.degree} onChange={v=>updateRow('education',i,{degree:v})} placeholder="e.g. Bachelor's"/><Field label="Field of study" value={entry.field} onChange={v=>updateRow('education',i,{field:v})} placeholder="e.g. Computer Science"/><Field label="Start date" type="month" value={entry.startDate} onChange={v=>updateRow('education',i,{startDate:v})}/><Field label="End date" type="month" value={entry.endDate} onChange={v=>updateRow('education',i,{endDate:v})}/></div></div>)}</div> : <Empty title="Add your education" description="Include your degree, school, and dates to complete your profile." action={<AddButton subtle onClick={() => addRow('education',{institution:'',degree:'',field:'',startDate:'',endDate:''})}>Add education</AddButton>}/>}</Section>
    <Section icon={BriefcaseBusiness} title="Work experience" description="Highlight the roles and impact that shaped your career." action={<AddButton onClick={() => addRow('experience',{company:'',title:'',employmentType:'',location:'',startDate:'',endDate:'',current:false,responsibilities:''})}>Add experience</AddButton>}>
      {fieldRows('experience').length ? <div className="entry-list">{fieldRows('experience').map((entry,i)=><div className="entry-card" key={entry.id || i}><div className="entry-top"><strong>Experience {i+1}</strong><RemoveButton onClick={() => removeRow('experience',i)}/></div><div className="form-grid"><Field label="Company" value={entry.company} onChange={v=>updateRow('experience',i,{company:v})} placeholder="Company name" required/><Field label="Job title" value={entry.title} onChange={v=>updateRow('experience',i,{title:v})} placeholder="Your role" required/><SelectField label="Employment type" value={entry.employmentType} onChange={v=>updateRow('experience',i,{employmentType:v})} options={['Full-time','Part-time','Contract','Internship','Freelance']} placeholder="Select type"/><Field label="Location" value={entry.location} onChange={v=>updateRow('experience',i,{location:v})} placeholder="City or remote"/><Field label="Start date" type="month" value={entry.startDate} onChange={v=>updateRow('experience',i,{startDate:v})}/>{!entry.current && <Field label="End date" type="month" value={entry.endDate} onChange={v=>updateRow('experience',i,{endDate:v})}/>}<label className="checkbox-line full-width"><input type="checkbox" checked={!!entry.current} onChange={e=>updateRow('experience',i,{current:e.target.checked,endDate:e.target.checked?'':entry.endDate})}/> I currently work here</label><div className="full-width"><TextArea label="Responsibilities and achievements" value={entry.responsibilities} onChange={v=>updateRow('experience',i,{responsibilities:v})} placeholder="Describe your impact and accomplishments." rows={3}/></div></div></div>)}</div> : <Empty title="Showcase your experience" description="Add the roles that have helped you grow professionally." action={<AddButton subtle onClick={() => addRow('experience',{company:'',title:'',employmentType:'',location:'',startDate:'',endDate:'',current:false,responsibilities:''})}>Add experience</AddButton>}/>}</Section>
    <Section icon={FileUser} title="Languages" description="Add each language and your proficiency level." action={<AddButton onClick={() => addRow('languages',{language:'',proficiency:'Intermediate'})}>Add language</AddButton>}>
      {fieldRows('languages').length ? <div className="entry-list compact-list">{fieldRows('languages').map((entry,i)=><div className="repeat-row" key={entry.id || i}><Field label="Language" value={entry.language} onChange={v=>updateRow('languages',i,{language:v})} placeholder="e.g. English" required/><SelectField label="Proficiency" value={entry.proficiency} onChange={v=>updateRow('languages',i,{proficiency:v})} options={['Beginner','Elementary','Intermediate','Upper-intermediate','Advanced','Fluent','Native','Bilingual']}/><RemoveButton onClick={() => removeRow('languages',i)} label="Remove language"/></div>)}</div> : <Empty title="Add languages you speak" description="Show employers how you communicate across languages." action={<AddButton subtle onClick={() => addRow('languages',{language:'',proficiency:'Intermediate'})}>Add language</AddButton>}/>}</Section>
    <RepeatSection title="Certificates" icon={FileUser} description="Share certifications that support your expertise." rows={fieldRows('certificates')} add={() => addRow('certificates',{name:'',organization:'',issueDate:'',expirationDate:'',url:''})} update={(i,v)=>updateRow('certificates',i,v)} remove={i=>removeRow('certificates',i)} fields={[['name','Certificate name'],['organization','Issuing organization'],['issueDate','Issue date','month'],['expirationDate','Expiration date','month'],['url','Credential URL','url']]}/>
    <RepeatSection title="Awards" icon={Sparkles} description="Recognize achievements that matter to your career." rows={fieldRows('awards')} add={() => addRow('awards',{name:'',organization:'',date:'',description:''})} update={(i,v)=>updateRow('awards',i,v)} remove={i=>removeRow('awards',i)} fields={[['name','Award name'],['organization','Issuing organization'],['date','Date received','month']]} descriptionField="Description"/>
    <Section icon={Search} title="Discover & Match" description="Create focused job searches that match different directions in your career." action={<AddButton onClick={() => go('/search/new')}>Add search</AddButton>}>
      {profile.searches.length ? <div className="search-list">{profile.searches.map(s=><article className="search-card" key={s.id}><button className="search-card-main" onClick={() => go(`/search/${s.id}`)}><span className="search-card-icon"><Search size={17}/></span><span className="search-card-copy"><strong>{s.name || 'Untitled search'}</strong><small>{searchSummary(s).join(' · ') || 'Add criteria to this search'}</small></span><span className="search-card-arrow"><ArrowRight size={16}/></span></button><button className="search-delete" aria-label={`Delete ${s.name || 'search'}`} onClick={() => { if (confirm(`Delete “${s.name || 'Untitled search'}”? This cannot be undone.`)) patch('searches',profile.searches.filter(x=>x.id!==s.id)) }}><Trash2 size={15}/></button></article>)}</div> : <Empty title="Set up your first job search" description="Build separate searches for the roles and locations you’re interested in." action={<AddButton subtle onClick={() => go('/search/new')}>Add Discover & Match</AddButton>}/>}</Section>
    <div className="form-actions"><button type="button" className="secondary-button" onClick={()=>go('/profiles')}>Cancel</button>{error && <div className="form-error" role="alert">{error}</div>}<button type="button" className="primary-button" disabled={busy} onClick={()=>onSave?.({ ...profile, id: profileId || profile.id || id(), name: profile.name || profile.personal?.headline || profile.personal?.name || 'My profile' })}>{busy ? 'Saving…' : 'Save profile'} <ArrowRight size={16}/></button></div>
  </div>
}

function ProfilesPage({ profiles, activeId, onSelect, onDelete, go }) {
  const atLimit = profiles.length >= 10
  const createButton = <button className="primary-button" onClick={()=>go('/profiles/create')} disabled={atLimit}><Plus size={16}/> Create New Profile</button>
  return <div className="page-wrap profiles-page">
    <PageTitle eyebrow="YOUR PROFESSIONAL IDENTITY" title="My Profiles" description="Manage the professional profiles you use for different career goals." right={profiles.length > 0 && createButton}/>
    {atLimit && <div className="limit-message" role="status">You have reached the maximum limit of 10 profiles.</div>}
    {!profiles.length ? <div className="profiles-empty"><div className="empty-icon"><UserRound size={22}/></div><h2>No professional profiles yet.</h2><p>Create your first profile to get started with job discovery and matching.</p><button className="primary-button" onClick={()=>go('/profiles/create')}><Plus size={16}/> Create Your First Profile</button></div> : <div className="profile-grid">{profiles.map(p=>{
      const active = p.id === activeId
      const title = p.personal?.headline || p.name || 'Professional profile'
      const summary = p.personal?.summary || 'Add a professional summary to highlight your experience and career goals.'
      const skillList = [...(p.skills?.programming || []), ...(p.skills?.technologies || []), ...(p.skills?.tools || [])]
      return <article className={`profile-card ${active?'is-active':''}`} key={p.id}>
        <div className="profile-card-top"><div className="profile-card-icon"><UserRound size={19}/></div>{active && <span className="active-badge"><Check size={13}/> Active</span>}</div>
        <h2>{p.name || p.personal?.name || 'Untitled profile'}</h2><strong className="profile-card-title">{title}</strong><p className="profile-card-summary">{summary}</p>
        <div className="profile-facts">{p.personal?.location && <span>{p.personal.location}</span>}<span>{(p.experience || []).length} experience</span><span>{skillList.length} skills</span></div>
        {skillList.length > 0 && <div className="profile-tags">{skillList.slice(0,4).map(skill=><span key={skill}>{skill}</span>)}</div>}
        <div className="profile-card-actions"><button className="secondary-button" onClick={()=>go(`/profiles/${p.id}/edit`)}><Pencil size={14}/> Edit</button><button className="secondary-button" onClick={()=>onDelete(p)}><Trash2 size={14}/> Delete</button><button className={active?'secondary-button selected-profile':'primary-button'} disabled={active} onClick={()=>onSelect(p)}>{active?<><Check size={14}/> Active Profile</>:'Select Profile'}</button></div>
      </article>
    })}</div>}
  </div>
}

function SelectField({ label, value, onChange, options, placeholder = 'Select an option' }) { return <label className="field"><span>{label}</span><span className="select-wrap"><select value={value || ''} onChange={e=>onChange(e.target.value)}><option value="">{placeholder}</option>{options.map(o=><option key={o} value={o}>{o}</option>)}</select><ChevronDown size={15}/></span></label> }
function RepeatSection({ title, icon, description, rows, add, update, remove, fields, descriptionField }) { return <Section icon={icon} title={title} description={description} action={<AddButton onClick={add}>Add {title.slice(0,-1).toLowerCase()}</AddButton>}>{rows.length ? <div className="entry-list">{rows.map((row,i)=><div className="entry-card" key={row.id || i}><div className="entry-top"><strong>{title.slice(0,-1)} {i+1}</strong><RemoveButton onClick={()=>remove(i)}/></div><div className="form-grid">{fields.map(([key,label,type])=><Field key={key} label={label} type={type || 'text'} value={row[key]} onChange={v=>update(i,{[key]:v})} placeholder={label} required={key==='name'}/>)}</div>{descriptionField && <div className="top-gap"><TextArea label={descriptionField} value={row.description} onChange={v=>update(i,{description:v})} placeholder="Add a few details about this award." rows={3}/></div>}</div>)}</div> : <Empty title={`Add ${title.toLowerCase()}`} description={`Include relevant ${title.toLowerCase()} in your profile.`} action={<AddButton subtle onClick={add}>Add {title.slice(0,-1).toLowerCase()}</AddButton>}/>}</Section> }

function SearchEditor({ search: initial, isNew, onChange, onSave, onCancel }) {
  const [draft, setDraft] = useState(() => ({...initial, titles:[...(initial.titles || [''])], workMode:[...(initial.workMode || [])]}))
  const [error,setError] = useState('')
  useEffect(() => setDraft({...initial,titles:[...(initial.titles || [''])],workMode:[...(initial.workMode || [])]}),[initial.id])
  const set = (key,value) => setDraft(d => ({...d,[key]:value}))
  const setTitle = (index,value) => set('titles',draft.titles.map((t,i)=>i===index?value:t))
  const toggleMode = (mode) => set('workMode',draft.workMode.includes(mode)?draft.workMode.filter(m=>m!==mode):[...draft.workMode,mode])
  const save = e => { e.preventDefault(); const clean={...draft,titles:draft.titles.map(t=>t.trim()).filter(Boolean)}; if(!clean.name.trim()){setError('Add a name for this search.');return} if(!clean.titles.length){setError('Add at least one job title.');return} if(Number(clean.hours||0)===0&&Number(clean.days||0)===0&&Number(clean.months||0)===0){setError('Set at least one value to define a posting window.');return} if([clean.hours,clean.days,clean.months].some(v=>Number(v)<0||!Number.isFinite(Number(v)))){setError('Posting window values must be non-negative numbers.');return} setError(''); onSave(clean,isNew) }
  return <div className="page-wrap editor-page"><button className="back-link" onClick={onCancel}><ArrowLeft size={16}/> Back to profile</button><PageTitle eyebrow="DISCOVER & MATCH" title={isNew?'Create a job search':'Edit job search'} description="Tell us what kind of opportunities you want to find."/>
    <form className="editor-form" onSubmit={save}>
      <Section icon={Search} title="Search details" description="Give this search a name and set your role criteria."><div className="form-grid"><Field label="Search name" value={draft.name} onChange={v=>set('name',v)} placeholder="e.g. Go Backend Developer" required/><div className="full-width"><div className="field"><span>Job titles<i> *</i></span><small className="field-hint">Add the different titles employers may use for this role.</small><div className="title-inputs">{draft.titles.map((t,i)=><div className="repeat-row title-row" key={i}><input aria-label={`Job title ${i+1}`} value={t} onChange={e=>setTitle(i,e.target.value)} placeholder="e.g. Backend Software Engineer" required={i===0}/>{draft.titles.length>1&&<button type="button" className="remove-icon" aria-label="Remove job title" onClick={()=>set('titles',draft.titles.filter((_,n)=>n!==i))}><X size={16}/></button>}</div>)}</div><AddButton subtle onClick={()=>set('titles',[...draft.titles,''])}>Add job title</AddButton></div></div><div className="full-width"><Field label="Technologies" value={draft.technologies} onChange={v=>set('technologies',v)} placeholder="Go, PostgreSQL, Redis, Kafka, Docker" hint="Separate technologies with commas."/></div><Field label="Region or location" value={draft.region} onChange={v=>set('region',v)} placeholder="e.g. India, Bengaluru" hint="Enter any region supported by your search provider."/><div className="field"><span>Work mode</span><div className="mode-options">{['Remote','Hybrid','On-site'].map(mode=><label className={`mode-option ${draft.workMode.includes(mode)?'selected':''}`} key={mode}><input type="checkbox" checked={draft.workMode.includes(mode)} onChange={()=>toggleMode(mode)}/><span className="mode-check">{draft.workMode.includes(mode)&&<Check size={12}/>}</span>{mode}</label>)}</div></div></div></Section>
      <Section icon={BriefcaseBusiness} title="Minimum salary" description="Set the lowest salary you would consider for this search."><div className="salary-row"><Field label="Minimum salary" type="number" min="0" value={draft.salary} onChange={v=>set('salary',v)} placeholder="400000"/><SelectField label="Currency" value={draft.currency} onChange={v=>set('currency',v)} options={['INR','USD','EUR','GBP','CAD','AUD']}/><SelectField label="Pay period" value={draft.period} onChange={v=>set('period',v)} options={['hour','month','year']}/></div></Section>
      <Section icon={Sparkles} title="Posted within" description="Choose how recently a job must have been posted. Set at least one value to define a time window."><div className="duration-grid">{[['hours','Hours'],['days','Days'],['months','Months']].map(([k,label])=><Field key={k} label={label} type="number" min="0" step="1" value={draft[k] ?? 0} onChange={v=>set(k,v)} placeholder="0" hint={k==='hours'?'0–23 hours':k==='days'?'Number of days':'Number of months'}/>)}</div><p className="duration-example">For example, 1 day and 10 hours searches for jobs posted within the last 34 hours.</p><p className="duration-note">Posting date filters depend on the job source. This setting is saved with your search.</p></Section>
      {error&&<div className="form-error" role="alert"><CircleHelp size={16}/>{error}</div>}<div className="form-actions"><button type="button" className="secondary-button" onClick={onCancel}>Cancel</button><button className="primary-button" type="submit">{isNew?'Create job search':'Save changes'} <ArrowRight size={16}/></button></div>
    </form>
  </div>
}

function Dashboard({ searches, go }) { return <div className="page-wrap dashboard-page"><PageTitle eyebrow="YOUR OPPORTUNITIES" title="Job dashboard" description="Keep track of the opportunities that match your career goals." right={<button className="secondary-button" disabled title="Job provider integration is not available yet"><Search size={16}/> Browse jobs</button>}/><div className="dashboard-stats"><div className="stat-card"><span>Configured searches</span><strong>{searches.length}</strong><small>Saved in this browser</small></div><div className="stat-card"><span>Jobs found</span><strong>—</strong><small>Job source not connected</small></div><div className="stat-card"><span>Top match</span><strong>—</strong><small>Matching score unavailable</small></div></div>{searches.length > 0 && <Section icon={Search} title="Your job searches" description="Open a search to review or update its criteria."><div className="search-list">{searches.map(s=><button className="dashboard-search" key={s.id} onClick={()=>go(`/search/${s.id}`)}><span className="search-card-icon"><Search size={16}/></span><span><strong>{s.name || 'Untitled search'}</strong><small>{(s.titles || []).filter(Boolean).join(', ') || 'No titles added'}</small></span><ArrowRight size={16}/></button>)}</div></Section>}<Section icon={BriefcaseBusiness} title="Matched jobs" description="Relevant jobs and their matching scores will appear here."><Empty title="No job matches yet" description="Job discovery isn’t connected to a job provider yet. Your searches are saved in this browser and will be ready to connect when a provider is available."/></Section><div className="integration-note"><Sparkles size={16}/><span><strong>Job discovery is not connected yet.</strong> Matching scores and listings will appear when a job source is integrated.</span></div></div> }

export default App

function AuthShell({ children }) { return <main className="auth-shell"><a className="brand auth-brand" href="/"><span className="brand-mark"><Sparkles size={17}/></span><span>magnas<span className="brand-light">hire</span></span></a>{children}</main> }
function AuthPage({ mode, busy, error, onSubmit, go }) {
  const registering = mode === 'register'
  const [username,setUsername] = useState('')
  const [password,setPassword] = useState('')
  const [confirm,setConfirm] = useState('')
  const [validation,setValidation] = useState('')
  const submit = e => { e.preventDefault(); setValidation(''); if (!username.trim() || !password) { setValidation('Enter your username and password.'); return } if (registering && password !== confirm) { setValidation('The passwords do not match.'); return } if (registering && password.length < 12) { setValidation('Use a password with at least 12 characters.'); return } onSubmit(username.trim(),password) }
  return <AuthShell><section className="auth-card"><div className="eyebrow">MAGNAS HIRE PILOT</div><h1>{registering ? 'Create your account' : 'Welcome back'}</h1><p>{registering ? 'Create an account to access your professional workspace.' : 'Sign in to continue to your professional workspace.'}</p><form onSubmit={submit}><label className="field"><span>Username</span><input autoComplete="username" value={username} onChange={e=>setUsername(e.target.value)} minLength="3" maxLength="32" required placeholder="Your username"/></label><label className="field"><span>Password</span><input type="password" autoComplete={registering ? 'new-password':'current-password'} value={password} onChange={e=>setPassword(e.target.value)} required placeholder="At least 12 characters"/></label>{registering && <label className="field"><span>Confirm password</span><input type="password" autoComplete="new-password" value={confirm} onChange={e=>setConfirm(e.target.value)} required placeholder="Enter password again"/></label>}{(validation || error) && <div className="form-error" role="alert">{validation || error}</div>}<button className="primary-button auth-submit" type="submit" disabled={busy}>{busy ? 'Please wait…' : registering ? 'Create account' : 'Sign in'} <ArrowRight size={16}/></button></form><div className="auth-switch">{registering ? 'Already have an account?' : 'New to Magnus?'} <button onClick={()=>go(registering?'/login':'/register')}>{registering?'Sign in':'Create an account'}</button></div></section></AuthShell>
}
