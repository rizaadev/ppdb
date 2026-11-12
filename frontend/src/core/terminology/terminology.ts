import { createContext, createElement, useContext, type ReactNode } from 'react';
import type { TerminologyPreset } from '@/core/types/domain';

export interface TerminologyDictionary {
  programAcronym: string;
  programTitle: string;
  institutionNoun: string;
  applicantNoun: string;
  guardianNoun: string;
  periodNoun: string;
  trackNoun: string;
  enrollmentNoun: string;
}

const PRESETS: Record<TerminologyPreset, TerminologyDictionary> = {
  spmb: {
    programAcronym: 'SPMB',
    programTitle: 'Sistem Penerimaan Murid Baru',
    institutionNoun: 'Sekolah',
    applicantNoun: 'Calon Murid',
    guardianNoun: 'Orang Tua / Wali',
    periodNoun: 'Tahun Ajaran',
    trackNoun: 'Jalur Penerimaan',
    enrollmentNoun: 'Daftar Ulang',
  },
  pmbm: {
    programAcronym: 'PMBM',
    programTitle: 'Penerimaan Murid Baru Madrasah',
    institutionNoun: 'Madrasah',
    applicantNoun: 'Calon Murid',
    guardianNoun: 'Orang Tua / Wali',
    periodNoun: 'Tahun Pelajaran',
    trackNoun: 'Jalur Madrasah',
    enrollmentNoun: 'Daftar Ulang',
  },
  mandiri: {
    programAcronym: 'Penerimaan Baru',
    programTitle: 'Portal Pendaftaran Murid & Santri Baru',
    institutionNoun: 'Lembaga / Pesantren',
    applicantNoun: 'Calon Murid / Santri',
    guardianNoun: 'Orang Tua / Wali Santri',
    periodNoun: 'Gelombang',
    trackNoun: 'Jalur & Program',
    enrollmentNoun: 'Daftar Ulang',
  },
};

const TerminologyContext = createContext<TerminologyDictionary>(PRESETS.spmb);

export function TerminologyProvider({
  preset,
  overrides,
  children,
}: {
  preset: TerminologyPreset;
  overrides?: Record<string, string>;
  children: ReactNode;
}) {
  const base = PRESETS[preset] ?? PRESETS.spmb;
  const merged: TerminologyDictionary = {
    ...base,
    ...(overrides as Partial<TerminologyDictionary>),
  };

  return createElement(TerminologyContext.Provider, { value: merged }, children);
}

export function useTerminology(): TerminologyDictionary {
  return useContext(TerminologyContext);
}

