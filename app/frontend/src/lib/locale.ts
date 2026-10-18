import { setupI18n } from '@lingui/core';
import { messages as de } from '$locales/de';
import { messages as en } from '$locales/en';
import { messages as fr } from '$locales/fr';
import { messages as it } from '$locales/it';

const messages = { en, de, fr, it };
type Locale = keyof typeof messages;

/** The first language of the OS the app has, else English. */
function osLocale(): Locale {
	const l = navigator.languages.map((l) => l.slice(0, 2)).find((l) => l in messages);
	return (l ?? 'en') as Locale;
}

/** The Lingui instance of the app, in the language of the OS. */
export const i18n = setupI18n({ locale: osLocale(), messages });
document.documentElement.lang = i18n.locale;
