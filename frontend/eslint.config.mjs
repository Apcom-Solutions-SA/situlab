import js from "@eslint/js";
import globals from "globals";

// Lint des scripts du frontend. Architecture à portée GLOBALE inter-fichiers (scripts classiques
// chargés dans l'ordre, pas de modules) : les symboles définis dans un fichier sont visibles des
// autres. Donc :
//   - `no-undef` désactivé (sinon chaque référence inter-fichiers serait un faux positif) ;
//   - `no-unused-vars` désactivé (une fonction « non utilisée » dans son fichier l'est depuis un
//     autre fichier ou un handler HTML : la règle n'est pas fiable ici).
// On garde le jeu « recommended » qui attrape les vrais défauts : clés dupliquées, code injoignable,
// conditions constantes, échappements inutiles, redéclarations, etc.
export default [
  js.configs.recommended,
  {
    files: ["dist/app/**/*.js"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "script",
      globals: { ...globals.browser, Chart: "readonly" },
    },
    rules: {
      "no-undef": "off",
      "no-unused-vars": "off",
      "no-empty": ["warn", { allowEmptyCatch: true }],
      "no-constant-condition": ["warn", { checkLoops: false }],
      "no-cond-assign": ["error", "except-parens"],
      eqeqeq: ["warn", "smart"],
    },
  },
];
