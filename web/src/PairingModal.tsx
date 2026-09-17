import { useEffect, useRef, useState } from 'react'
import { statusLabel, type ConnectionState, type Instance } from './api'
import { LoadingState } from './LoadingState'

function QRCode({ source }: { source: string }) {
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  return (
    <div className="qrStage">
      <div className={`qrFrame ${loaded ? 'qrReady' : ''}`} aria-busy={!loaded && !failed}>
        {!failed && <img src={`${source}&attempt=${attempt}`} alt="QR Code para conectar o WhatsApp"
          onLoad={() => setLoaded(true)} onError={() => setFailed(true)} />}
        {!loaded && <div className="qrPlaceholder">
          {failed ? <div role="alert"><p>Não foi possível carregar o QR Code.</p>
            <button className="secondaryButton" onClick={() => { setFailed(false); setAttempt((n) => n + 1) }}>Tentar novamente</button>
          </div> : <LoadingState title="Carregando QR Code" />}
        </div>}
      </div>
      <h3>Escaneie com o WhatsApp</h3>
      <p>No celular, abra <strong>Aparelhos conectados</strong><br />e toque em Conectar aparelho.</p>
      <span className="qrAutoRefresh"><span />O código é atualizado automaticamente</span>
    </div>
  )
}

export function PairingModal({ instance, connection, hasShownQR, starting, onClose }: {
  instance: Instance
  connection: ConnectionState | null
  hasShownQR: boolean
  starting: boolean
  onClose: () => Promise<void>
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [closing, setClosing] = useState(false)
  const [closeError, setCloseError] = useState('')
  const connected = connection?.status === 'connected'
  const failed = ['error', 'disconnected', 'logged_out'].includes(connection?.status ?? '')
  const step = connected ? 3 : hasShownQR || connection?.qrAvailable ? 2 : 1
  const version = connection?.qrExpiresAt ?? 'current'

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  async function close() {
    if (closing || starting) return
    setClosing(true)
    try { await onClose() } catch {
      setCloseError('Não foi possível encerrar a conexão. Tente novamente.')
      setClosing(false)
    }
  }

  return (
    <dialog ref={dialog} className="pairingModal" aria-labelledby="pairing-title"
      onCancel={(event) => { event.preventDefault(); void close() }}>
      <header className="modalHeader">
        <div><p className="eyebrow">CONECTAR WHATSAPP</p><h2 id="pairing-title">{instance.name}</h2></div>
        <button className="closeButton" type="button" aria-label="Fechar pareamento" disabled={closing || starting} onClick={() => void close()}>×</button>
      </header>
      <ol className="pairingSteps" aria-label="Etapas do pareamento">
        {['Preparar', 'Escanear', 'Conectar'].map((label, index) => <li key={label}
          className={step > index + 1 ? 'stepComplete' : ''} aria-current={step === index + 1 ? 'step' : undefined}>
          <span>{step > index + 1 ? '✓' : index + 1}</span>{label}
        </li>)}
      </ol>
      <div className="pairingBody">
        {connected ? <div className="connectionSuccess" role="status">
          <span className="successMark" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m6 12 4 4 8-8" /></svg></span>
          <h3>WhatsApp conectado</h3><p>Tudo pronto. Sua sessão está salva<br />e a instância já pode ser utilizada.</p>
        </div> : failed ? <div className="connectionError" role="alert">
          <span className="errorMark" aria-hidden="true">!</span><h3>{connection?.status === 'error' ? 'Não foi possível conectar' : 'Pareamento encerrado'}</h3>
          <p>{connection?.lastError || 'Feche esta janela e clique em Conectar para tentar novamente.'}</p>
        </div> : connection?.qrAvailable ? <QRCode key={version} source={`/api/instances/${instance.id}/qr?v=${encodeURIComponent(version)}`} />
          : <LoadingState title={hasShownQR && connection?.status === 'connecting' ? 'Finalizando conexão' : 'Preparando conexão'}
            description={hasShownQR && connection?.status === 'connecting' ? 'Validando sua sessão com o WhatsApp. Só mais um instante.' : 'Estamos preparando seu QR Code. Ele aparecerá automaticamente.'} />}
      </div>
      {closeError && <p className="formError pairingCloseError" role="alert">{closeError}</p>}
      <footer className="modalFooter">
        <span className={`badge badge-${connection?.status ?? 'connecting'}`}>{statusLabel(connection?.status ?? 'connecting')}</span>
        <button className={connected ? 'primaryButton' : 'secondaryButton'} type="button" aria-busy={closing} disabled={closing || starting} onClick={() => void close()}>
          {closing ? 'Encerrando…' : connected ? 'Concluir' : 'Cancelar'}
        </button>
      </footer>
    </dialog>
  )
}
