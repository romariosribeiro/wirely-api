/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
import english from "./translations.en.json";
export type Language = "pt" | "en";
type LanguageContextValue = {
  language: Language;
  setLanguage: (language: Language) => void;
};
const LanguageContext = createContext<LanguageContextValue | null>(null);
const originalTexts = new WeakMap<Text, string>();
const originalAttributes = new WeakMap<Element, Map<string, string>>();
const translatedAttributes = ["aria-label", "placeholder", "title"];
function translateCore(value: string) {
  const exact = (english as Record<string, string>)[value];
  if (exact) return exact;
  const patterns: Array<[RegExp, (...matches: string[]) => string]> = [
    [/^Criada em (.+)$/, (_, date) => `Created on ${date}`],
    [
      /^(\d+) recebidas · (\d+) enviadas$/,
      (_, received, sent) => `${received} received · ${sent} sent`,
    ],
    [
      /^(\d+) falhas nas últimas 24h$/,
      (_, failures) => `${failures} failures in the last 24h`,
    ],
    [/^Atividade · (.+)$/, (_, name) => `Activity · ${name}`],
    [/^Testar envio · (.+)$/, (_, name) => `Test message · ${name}`],
    [/^Conversas · (.+)$/, (_, name) => `Conversations · ${name}`],
    [/^(\d+) registros$/, (_, count) => `${count} records`],
    [/^(\d+) registro$/, (_, count) => `${count} record`],
    [/^Página (\d+) de (\d+)$/, (_, page, total) => `Page ${page} of ${total}`],
    [/^Papel de (.+)$/, (_, username) => `Role for ${username}`],
    [
      /^Redefinir senha · (.+)$/,
      (_, username) => `Reset password · ${username}`,
    ],
    [/^(\d+) ROTAS$/, (_, count) => `${count} ROUTES`],
    [
      /^Desconectar (.+) do WhatsApp\?$/,
      (_, name) => `Disconnect ${name} from WhatsApp?`,
    ],
    [
      /^Desconectar "(.+)" do WhatsApp\?$/,
      (_, name) => `Disconnect "${name}" from WhatsApp?`,
    ],
    [
      /^Excluir "(.+)" e sua sessão local\? Esta ação não pode ser desfeita\.$/,
      (_, name) =>
        `Delete "${name}" and its local session? This action cannot be undone.`,
    ],
    [
      /^Criar um backup e atualizar para (.+)\? O Wirely reiniciará automaticamente\.$/,
      (_, version) =>
        `Create a backup and update to ${version}? Wirely will restart automatically.`,
    ],
    [/^Atualizado às (.+)$/, (_, time) => `Updated at ${time}`],
    [
      /^Automático a cada (.+)h · retenção de (.+) arquivos$/,
      (_, interval, retention) =>
        `Automatic every ${interval}h · retention of ${retention} files`,
    ],
    [/^Reação (.+)$/, (_, reaction) => `Reaction ${reaction}`],
    [/^Último acesso (.+)$/, (_, date) => `Last signed in ${date}`],
    [/^Nova senha de (.+)$/, (_, username) => `New password for ${username}`],
  ];
  for (const [pattern, replacement] of patterns) {
    const match = value.match(pattern);
    if (match) return replacement(...match);
  }
  const replacements: Array<[string, string]> = [
    ["SUA_SENHA", "YOUR_PASSWORD"],
    ["seu-dominio.com", "your-domain.com"],
    ["Olá pelo Wirely", "Hello from Wirely"],
    ["Olá pela fila", "Hello from the queue"],
    ["Texto corrigido", "Corrected text"],
    [
      "# Requer sessão administrativa salva em wirely.cookies",
      "# Requires an administrative session saved in wirely.cookies",
    ],
    [
      "# Reutilize session nas rotas administrativas.",
      "# Reuse session for administrative routes.",
    ],
    [
      "# A senha é enviada somente no login:",
      "# The password is sent only during login:",
    ],
    ["# Enfileira um evento webhook.test", "# Enqueue a webhook.test event"],
    ["# Consulte o eventId retornado", "# Check the returned eventId"],
    ["# Histórico de tentativas", "# Delivery attempt history"],
    [
      "# Imagem, vídeo, áudio, documento e figurinha usam /api/queue/media:",
      "# Image, video, audio, document, and sticker use /api/queue/media:",
    ],
    [
      "# A resposta 202 informa o ID do job. Consulte depois:",
      "# The 202 response includes the job ID. Check it later:",
    ],
  ];
  let translated = value;
  for (const [source, target] of replacements) {
    translated = translated.replaceAll(source, target);
  }
  return translated;
}
export function translateText(value: string, language: Language = "en") {
  if (language === "pt" || !value.trim()) return value;
  const leading = value.match(/^\s*/)?.[0] ?? "";
  const trailing = value.match(/\s*$/)?.[0] ?? "";
  const core = value.slice(leading.length, value.length - trailing.length);
  return `${leading}${translateCore(core)}${trailing}`;
}
function applyText(node: Text, language: Language, refreshOriginal: boolean) {
  const current = node.data;
  let original = originalTexts.get(node);
  if (original === undefined) {
    original = current;
    originalTexts.set(node, original);
  } else if (refreshOriginal) {
    const expected = language === "en" ? translateText(original) : original;
    if (current !== expected) {
      original = current;
      originalTexts.set(node, original);
    }
  }
  const desired = language === "en" ? translateText(original) : original;
  if (node.data !== desired) node.data = desired;
}
function applyAttributes(
  element: Element,
  language: Language,
  refreshOriginal: boolean,
) {
  let originals = originalAttributes.get(element);
  if (!originals) {
    originals = new Map();
    originalAttributes.set(element, originals);
  }
  for (const attribute of translatedAttributes) {
    const current = element.getAttribute(attribute);
    if (current === null) continue;
    let original = originals.get(attribute);
    if (original === undefined) {
      original = current;
      originals.set(attribute, original);
    } else if (refreshOriginal) {
      const expected = language === "en" ? translateText(original) : original;
      if (current !== expected) {
        original = current;
        originals.set(attribute, original);
      }
    }
    const desired = language === "en" ? translateText(original) : original;
    if (current !== desired) element.setAttribute(attribute, desired);
  }
}
function applyTree(root: Node, language: Language, refreshOriginal = false) {
  if (root instanceof Text) applyText(root, language, refreshOriginal);
  if (root instanceof Element) applyAttributes(root, language, refreshOriginal);
  const walker = document.createTreeWalker(
    root,
    NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT,
  );
  let node = walker.nextNode();
  while (node) {
    if (node instanceof Text) applyText(node, language, refreshOriginal);
    else if (node instanceof Element)
      applyAttributes(node, language, refreshOriginal);
    node = walker.nextNode();
  }
}
export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(() =>
    window.localStorage.getItem("wirely.language") === "en" ? "en" : "pt",
  );
  const setLanguage = useCallback((next: Language) => {
    window.localStorage.setItem("wirely.language", next);
    setLanguageState(next);
  }, []);
  useLayoutEffect(() => {
    document.documentElement.lang = language === "en" ? "en" : "pt-BR";
    applyTree(document.body, language);
    const nativeConfirm = window.confirm.bind(window);
    window.confirm = (message?: string) =>
      nativeConfirm(
        language === "en" ? translateText(message ?? "") : (message ?? ""),
      );
    const observer = new MutationObserver((records) => {
      for (const record of records) {
        if (record.type === "characterData")
          applyTree(record.target, language, true);
        else if (record.type === "attributes")
          applyTree(record.target, language, true);
        else for (const node of record.addedNodes) applyTree(node, language);
      }
    });
    observer.observe(document.body, {
      attributes: true,
      attributeFilter: translatedAttributes,
      characterData: true,
      childList: true,
      subtree: true,
    });
    return () => {
      observer.disconnect();
      window.confirm = nativeConfirm;
    };
  }, [language]);
  const value = useMemo(
    () => ({ language, setLanguage }),
    [language, setLanguage],
  );
  return (
    <LanguageContext.Provider value={value}>
      {children}
    </LanguageContext.Provider>
  );
}
export function useLanguage() {
  const context = useContext(LanguageContext);
  if (!context)
    throw new Error("useLanguage must be used inside LanguageProvider");
  return context;
}
