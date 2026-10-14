import { createContext, useContext, useState, type ReactNode } from "react";

type Language = "en" | "zh";
const LanguageContext = createContext<{
  language: Language;
  setLanguage: (v: Language) => void;
}>({ language: "en", setLanguage: () => {} });

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, select] = useState<Language>(() => {
    const saved = localStorage.getItem("taskbelay-language");
    return saved === "zh" || saved === "en"
      ? saved
      : navigator.language.startsWith("zh")
        ? "zh"
        : "en";
  });
  function setLanguage(value: Language) {
    select(value);
    localStorage.setItem("taskbelay-language", value);
    document.documentElement.lang = value === "zh" ? "zh-CN" : "en";
  }
  return (
    <LanguageContext.Provider value={{ language, setLanguage }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage() {
  const context = useContext(LanguageContext);
  return {
    ...context,
    label: (english: string, chinese: string) =>
      context.language === "zh" ? chinese : english,
  };
}
