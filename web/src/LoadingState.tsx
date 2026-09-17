export function LoadingState({ title, description }: { title: string; description?: string }) {
  return (
    <div className="connectionLoading" role="status" aria-live="polite">
      <div className="loadingOrbit" aria-hidden="true"><span className="spinner" /><span className="loadingCore" /></div>
      <h3>{title}</h3>
      {description && <p>{description}</p>}
      <div className="loadingDots" aria-hidden="true"><i /><i /><i /></div>
    </div>
  )
}
