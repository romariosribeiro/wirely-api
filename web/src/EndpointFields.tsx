import { useEffect, useState } from 'react'
import { useLanguage } from './i18n'

type Document = Record<string, unknown>
type Schema = {
  $ref?: string
  type?: string
  format?: string
  description?: string
  enum?: unknown[]
  required?: string[]
  properties?: Record<string, Schema>
  items?: Schema
  allOf?: Schema[]
}

type Field = {
  name: string
  location: string
  required: boolean
  description: string
  type: string
}

let documentPromise: Promise<Document> | undefined

function openAPIDocument() {
  documentPromise ??= fetch('/openapi.json').then(async (response) => {
    if (!response.ok) throw new Error('OpenAPI unavailable')
    return response.json() as Promise<Document>
  })
  return documentPromise
}

function resolveRef(document: Document, value: unknown): Schema {
  const schema = (value ?? {}) as Schema
  if (!schema.$ref?.startsWith('#/')) return schema
  return schema.$ref.slice(2).split('/').reduce<unknown>((current, part) => {
    return (current as Record<string, unknown>)?.[part.replace(/~1/g, '/').replace(/~0/g, '~')]
  }, document) as Schema
}

function mergedSchema(document: Document, value: unknown): Schema {
  const schema = resolveRef(document, value)
  if (!schema.allOf) return schema
  return schema.allOf.reduce<Schema>((result, part) => {
    const resolved = mergedSchema(document, part)
    return {
      ...result,
      ...resolved,
      required: [...new Set([...(result.required ?? []), ...(resolved.required ?? [])])],
      properties: { ...(result.properties ?? {}), ...(resolved.properties ?? {}) },
    }
  }, {})
}

function typeLabel(document: Document, value: unknown): string {
  const schema = mergedSchema(document, value)
  if (schema.enum?.length) return schema.enum.map(String).join(' | ')
  if (schema.type === 'array') return `array<${typeLabel(document, schema.items)}>`
  return schema.format ? `${schema.type ?? 'string'} (${schema.format})` : (schema.type ?? 'object')
}

function fieldsFor(document: Document, path: string, method: string): Field[] {
  const cleanPath = path.split('?')[0]
  const pathItem = ((document.paths as Record<string, unknown> | undefined)?.[cleanPath] ?? {}) as Record<string, unknown>
  const operation = (pathItem[method.toLowerCase()] ?? {}) as Record<string, unknown>
  const parameters = [...((pathItem.parameters as unknown[]) ?? []), ...((operation.parameters as unknown[]) ?? [])]
  const fields: Field[] = parameters.map((value) => {
    const parameter = resolveRef(document, value) as Schema & { name?: string; in?: string; required?: boolean; schema?: Schema }
    return {
      name: parameter.name ?? 'parameter',
      location: parameter.in ?? 'parameter',
      required: Boolean(parameter.required),
      description: parameter.description ?? '',
      type: typeLabel(document, parameter.schema),
    }
  })

  let requestBody = operation.requestBody
  if (requestBody) {
    requestBody = resolveRef(document, requestBody)
    const body = requestBody as Schema & { content?: Record<string, { schema?: Schema }> }
    const content = body.content ?? {}
    const media = content['application/json'] ?? content['multipart/form-data'] ?? Object.values(content)[0]
    const schema = mergedSchema(document, media?.schema)
    const required = new Set(schema.required ?? [])
    Object.entries(schema.properties ?? {}).forEach(([name, property]) => {
      const resolved = mergedSchema(document, property)
      fields.push({
        name,
        location: content['multipart/form-data'] ? 'form' : 'body',
        required: required.has(name),
        description: resolved.description ?? '',
        type: typeLabel(document, resolved),
      })
    })
  }
  return fields
}

export function EndpointFields({ path, method, hidden = false }: { path: string; method: string; hidden?: boolean }) {
  const { language } = useLanguage()
  const english = language === 'en'
  const [fields, setFields] = useState<Field[]>([])

  useEffect(() => {
    let active = true
    void openAPIDocument().then((document) => {
      if (active) setFields(fieldsFor(document, path, method))
    }).catch(() => {
      if (active) setFields([])
    })
    return () => { active = false }
  }, [method, path])

  if (hidden || fields.length === 0) return null
  const required = fields.filter((field) => field.required)
  const optional = fields.filter((field) => !field.required)

  const render = (items: Field[]) => <ul>{items.map((field) => <li key={`${field.location}-${field.name}`}>
    <code>{field.name}</code>
    <span>{field.type} · {field.location}{field.description ? ` — ${field.description}` : ''}</span>
  </li>)}</ul>

  return <div className="docsFieldGuide">
    <div>
      <strong>{english ? 'Required fields' : 'Campos obrigatórios'}</strong>
      {required.length ? render(required) : <p>{english ? 'None.' : 'Nenhum.'}</p>}
    </div>
    <div>
      <strong>{english ? 'Optional fields' : 'Campos opcionais'}</strong>
      {optional.length ? render(optional) : <p>{english ? 'None.' : 'Nenhum.'}</p>}
    </div>
  </div>
}
