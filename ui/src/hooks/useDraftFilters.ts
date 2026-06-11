/**
 * @module ui/src/hooks/useDraftFilters.ts
 */
import { useCallback, useRef, useState } from "react";

export function useDraftFilters<T extends Record<string, string>>(initialValues: T) {
  const initialValuesRef = useRef(initialValues);

  const cloneInitialValues = useCallback(
    () => ({ ...initialValuesRef.current }) as T,
    [],
  );

  const [draftValues, setDraftValues] = useState<T>(() => cloneInitialValues());
  const [appliedValues, setAppliedValues] = useState<T>(() => cloneInitialValues());

  const setDraftValue = useCallback((name: string, value: string) => {
    setDraftValues((prev) => ({ ...prev, [name]: value } as T));
  }, []);

  const applyValues = useCallback((values: Record<string, string>) => {
    setAppliedValues(values as T);
  }, []);

  const resetValues = useCallback(() => {
    const nextValues = cloneInitialValues();
    setDraftValues(nextValues);
    setAppliedValues(nextValues);
  }, [cloneInitialValues]);

  return {
    draftValues,
    appliedValues,
    setDraftValue,
    applyValues,
    resetValues,
  };
}
