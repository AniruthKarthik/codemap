export interface LineRange {
  Start: number;
  End: number;
}

export interface Highlight {
  Start: number;
  End: number;
  Reason: string;
}

export interface FileSlice {
  FilePath: string;
  Ranges: LineRange[];
  Highlights: Highlight[];
}

export interface LearningStep {
  Order: number;
  File: string;
  Purpose: string;
  LearningObjective: string;
  KeySymbols: string[];
  Unlocks: string[];
  Score: number;
  Slice: FileSlice;
}

export interface Analysis {
  repository: string;
  steps: LearningStep[];
}

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}
